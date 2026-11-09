//go:build windows

package desktop

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

	"github.com/thescottbot/punchline/internal/app"
	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/icon"
	"github.com/thescottbot/punchline/internal/product"
)

// The Win32 message loop and every window, hotkey and hook call must stay on
// the thread that created the window: the main thread.
func init() { runtime.LockOSThread() }

const (
	className      = "Punchline.TrayWindow"
	mutexName      = `Local\Punchline.SingleInstance`
	trayID         = 1
	timerDevice    = 1
	timerCapture   = 2
	timerUnhook    = 3
	deviceDebounce = 700 * time.Millisecond
	captureTimeout = 15 * time.Second
)

// Shell is the Windows desktop integration: a hidden window owning the tray
// icon, hotkeys and the temporary key-capture hook.
type Shell struct {
	cfg     Config
	log     *log.Logger
	exe     string
	hinst   windows.Handle
	hwnd    windows.HWND
	mutex   windows.Handle
	reg     *winRegistrar
	core    *app.Core
	taskbar uint32

	icons    map[icon.Style]windows.Handle
	iconSize int
	trayUp   bool
	lastTip  string
	lastIcon icon.Style

	wndProc  uintptr
	hookProc uintptr
	hook     uintptr
	capture  struct {
		active    bool
		action    hotkey.Action
		pressed   uint32
		finishing bool
	}
	shutdownDone bool
}

// New claims the single-instance lock and creates the hidden window.
func New(cfg Config) (*Shell, error) {
	cfg.defaults()
	s := &Shell{cfg: cfg, log: cfg.Logger, icons: map[icon.Style]windows.Handle{}}

	m, err := windows.CreateMutex(nil, false, utf16Ptr(mutexName))
	if err == windows.ERROR_ALREADY_EXISTS {
		if m != 0 {
			windows.CloseHandle(m)
		}
		if h := findWindow(); h != 0 {
			postMessage(h, wmAlreadyRunning, 0, 0)
		}
		return nil, ErrAlreadyRunning
	}
	if err != nil {
		return nil, fmt.Errorf("single-instance lock: %w", err)
	}
	s.mutex = m

	procSetProcessDpiAwarenessCtx.Call(^uintptr(3)) // PER_MONITOR_AWARE_V2 (-4)
	if exe, err := os.Executable(); err == nil {
		s.exe = exe
	}
	h, _, _ := procGetModuleHandleW.Call(0)
	s.hinst = windows.Handle(h)
	s.wndProc = windows.NewCallback(s.windowProc)
	s.hookProc = windows.NewCallback(s.keyboardProc)
	r, _, _ := procRegisterWindowMessageW.Call(uintptr(unsafe.Pointer(utf16Ptr("TaskbarCreated"))))
	s.taskbar = uint32(r)

	wc := wndClassEx{
		Size:      uint32(unsafe.Sizeof(wndClassEx{})),
		WndProc:   s.wndProc,
		Instance:  s.hinst,
		ClassName: utf16Ptr(className),
	}
	if a, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); a == 0 {
		s.release()
		return nil, fmt.Errorf("RegisterClassEx: %w", err)
	}
	// A hidden top-level (not message-only) window, so it receives the
	// TaskbarCreated and WM_DEVICECHANGE broadcasts. It is never shown.
	hw, _, err := procCreateWindowExW.Call(wsExToolWindow,
		uintptr(unsafe.Pointer(utf16Ptr(className))), uintptr(unsafe.Pointer(utf16Ptr(product.DisplayName))),
		wsPopup, 0, 0, 0, 0, 0, 0, uintptr(s.hinst), 0)
	if hw == 0 {
		s.release()
		return nil, fmt.Errorf("CreateWindowEx: %w", err)
	}
	s.hwnd = windows.HWND(hw)
	s.reg = newRegistrar(s.hwnd)
	return s, nil
}

// Registrar returns the OS hotkey registrar for the core.
func (s *Shell) Registrar() app.Registrar { return s.reg }

// Changed may be called from any goroutine when core state changes.
func (s *Shell) Changed() {
	if s.hwnd != 0 {
		postMessage(s.hwnd, wmStateChanged, 0, 0)
	}
}

// Run shows the tray icon, starts the core and runs the message loop until
// Quit. notices are shown once as a notification (e.g. settings problems).
func (s *Shell) Run(core *app.Core, notices []string) error {
	s.core = core
	s.addTray()
	if err := core.Start(); err != nil {
		s.notify("Hotkey problem", err.Error(), true)
	} else if len(notices) > 0 {
		s.notify("Settings", strings.Join(notices, "\n"), true)
	}
	s.log.Printf("running; exe %s", s.exe)
	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	s.shutdown()
	s.release()
	return nil
}

// RequestQuit asks a running instance to quit. It reports whether one was
// found.
func RequestQuit() bool {
	h := findWindow()
	if h != 0 {
		postMessage(h, wmClose, 0, 0)
	}
	return h != 0
}

func findWindow() windows.HWND {
	h, _, _ := procFindWindowW.Call(uintptr(unsafe.Pointer(utf16Ptr(className))), 0)
	return windows.HWND(h)
}

func (s *Shell) shutdown() {
	if s.shutdownDone {
		return
	}
	s.shutdownDone = true
	s.stopCapture()
	if s.core != nil {
		s.core.Shutdown()
	}
	s.removeTray()
}

func (s *Shell) release() {
	for k, h := range s.icons {
		procDestroyIcon.Call(uintptr(h))
		delete(s.icons, k)
	}
	if s.mutex != 0 {
		windows.CloseHandle(s.mutex)
		s.mutex = 0
	}
}

func (s *Shell) windowProc(hwnd, m, wp, lp uintptr) (ret uintptr) {
	defer func() {
		if r := recover(); r != nil {
			s.log.Printf("panic in window procedure: %v\n%s", r, debug.Stack())
			ret = 0
		}
	}()
	switch uint32(m) {
	case wmHotkey:
		if a, ok := s.reg.action(wp); ok && s.core != nil {
			s.core.Press(a)
		}
		return 0
	case wmTrayCallback:
		switch loword(lp) {
		case wmContextMenu, ninSelect, ninKeySelect:
			s.showMenu(int32(int16(loword(wp))), int32(int16(hiword(wp))))
		}
		return 0
	case wmStateChanged:
		s.updateTray()
		return 0
	case wmCaptured:
		s.onCaptured(uint32(wp))
		return 0
	case wmCaptureKeyUp:
		if s.capture.finishing {
			s.stopCapture()
		}
		return 0
	case wmTimer:
		procKillTimer.Call(hwnd, wp)
		switch wp {
		case timerDevice:
			if s.core != nil {
				s.core.TriggerRescan()
			}
		case timerCapture:
			if s.capture.active && !s.capture.finishing {
				s.core.CancelRebind()
				s.stopCapture()
				s.notify("Rebind cancelled", "No key was pressed. Bindings are unchanged.", false)
			}
		case timerUnhook:
			s.stopCapture()
		}
		return 0
	case wmDeviceChange:
		switch wp {
		case dbtDevNodesChanged, dbtDeviceArrival, dbtDeviceRemoveComplete:
			procSetTimer.Call(hwnd, timerDevice, uintptr(deviceDebounce/time.Millisecond), 0)
		}
		return 1
	case wmAlreadyRunning:
		s.notify(product.DisplayName, "Already running. Use the tray icon to change settings.", false)
		return 0
	case wmQueryEndSession:
		return 1
	case wmEndSession:
		if wp != 0 {
			s.log.Printf("session ending")
			s.shutdown()
		}
		return 0
	case wmClose:
		s.log.Printf("quit requested")
		s.shutdown()
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	if s.taskbar != 0 && uint32(m) == s.taskbar {
		s.log.Printf("taskbar restarted; restoring tray icon")
		s.trayUp = false
		s.addTray()
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, m, wp, lp)
	return r
}

// ---- tray ---------------------------------------------------------------

func (s *Shell) iconFor(st icon.Style) windows.Handle {
	if h, ok := s.icons[st]; ok {
		return h
	}
	if s.iconSize == 0 {
		n, _, _ := procGetSystemMetrics.Call(smCxSmIcon)
		s.iconSize = int(n)
		if s.iconSize < 16 {
			s.iconSize = 16
		}
	}
	h, err := createIcon(s.iconSize, icon.Render(s.iconSize, st))
	if err != nil {
		s.log.Printf("creating icon: %v", err)
	}
	s.icons[st] = h
	return h
}

func (s *Shell) nid() notifyIconData {
	return notifyIconData{CbSize: uint32(unsafe.Sizeof(notifyIconData{})), Wnd: s.hwnd, ID: trayID}
}

func (s *Shell) addTray() {
	sn := s.snapshot()
	st := styleFor(sn)
	n := s.nid()
	n.Flags = nifMessage | nifIcon | nifTip | nifShowTip
	n.CallbackMessage = wmTrayCallback
	n.Icon = s.iconFor(st)
	tip := Tooltip(sn)
	copyUTF16(n.Tip[:], tip)
	if r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&n))); r == 0 {
		// Explorer may not be ready yet at login; TaskbarCreated will retry.
		s.log.Printf("tray icon not added yet; waiting for the taskbar")
		return
	}
	n.Version = notifyIconV4
	procShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&n)))
	s.trayUp, s.lastTip, s.lastIcon = true, tip, st
}

func (s *Shell) updateTray() {
	if !s.trayUp {
		return
	}
	sn := s.snapshot()
	st, tip := styleFor(sn), Tooltip(sn)
	if st == s.lastIcon && tip == s.lastTip {
		return
	}
	n := s.nid()
	n.Flags = nifIcon | nifTip | nifShowTip
	n.Icon = s.iconFor(st)
	copyUTF16(n.Tip[:], tip)
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&n)))
	s.lastTip, s.lastIcon = tip, st
}

func (s *Shell) removeTray() {
	if !s.trayUp {
		return
	}
	n := s.nid()
	procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&n)))
	s.trayUp = false
}

// notify shows a silent tray notification. Used for results of actions the
// user just took and for problems that need attention.
func (s *Shell) notify(title, text string, warn bool) {
	s.log.Printf("notice: %s: %s", title, text)
	if !s.trayUp {
		return
	}
	n := s.nid()
	n.Flags = nifInfo
	n.InfoFlags = niifInfo | niifNoSound
	if warn {
		n.InfoFlags = niifWarning | niifNoSound
	}
	copyUTF16(n.InfoTitle[:], title)
	copyUTF16(n.Info[:], text)
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&n)))
}

func (s *Shell) snapshot() app.Snapshot {
	if s.core == nil {
		return app.Snapshot{Bindings: hotkey.DefaultBindings()}
	}
	return s.core.Snapshot()
}

func styleFor(sn app.Snapshot) icon.Style {
	if Armed(sn) {
		return icon.Active
	}
	return icon.Inactive
}

// ---- menu ---------------------------------------------------------------

func (s *Shell) showMenu(x, y int32) {
	if s.core == nil {
		return
	}
	items := BuildMenu(s.core.Snapshot(), startupState(s.exe), PermissionState{})
	var cmds []Command
	hm := buildNativeMenu(items, &cmds)
	defer procDestroyMenu.Call(hm)
	if x == 0 && y == 0 {
		var p point
		procGetCursorPos.Call(uintptr(unsafe.Pointer(&p)))
		x, y = p.X, p.Y
	}
	procSetForegroundWindow.Call(uintptr(s.hwnd))
	id, _, _ := procTrackPopupMenu.Call(hm, tpmReturnCmd|tpmNoNotify|tpmRightButton,
		uintptr(x), uintptr(y), 0, uintptr(s.hwnd), 0)
	postMessage(s.hwnd, wmNull, 0, 0)
	if id == 0 || int(id) > len(cmds) {
		return
	}
	s.run(cmds[id-1])
}

func buildNativeMenu(items []Item, cmds *[]Command) uintptr {
	hm, _, _ := procCreatePopupMenu.Call()
	for _, it := range items {
		text := strings.ReplaceAll(it.Text, "&", "&&")
		flags := uintptr(mfString)
		if it.Disabled {
			flags |= mfGrayed
		}
		if it.Checked {
			flags |= mfChecked
		}
		switch it.Kind {
		case KindSeparator:
			procAppendMenuW.Call(hm, mfSeparator, 0, 0)
		case KindSubmenu:
			sub := buildNativeMenu(it.Children, cmds)
			procAppendMenuW.Call(hm, flags|mfPopup, sub, uintptr(unsafe.Pointer(utf16Ptr(text))))
		case KindLabel:
			procAppendMenuW.Call(hm, flags|mfGrayed, 0, uintptr(unsafe.Pointer(utf16Ptr(text))))
		default:
			*cmds = append(*cmds, it.Cmd)
			procAppendMenuW.Call(hm, flags, uintptr(len(*cmds)), uintptr(unsafe.Pointer(utf16Ptr(text))))
		}
	}
	return hm
}

func (s *Shell) run(c Command) {
	switch c.Kind {
	case CmdToggleHotkeys:
		on := !s.core.Snapshot().Enabled
		if err := s.core.SetEnabled(on); err != nil {
			s.notify("Hotkey problem", err.Error(), true)
		}
	case CmdRebind:
		s.startCapture(c.Action)
	case CmdSelectCamera:
		s.core.SelectCamera(c.Camera)
	case CmdToggleStartup:
		on := !startupState(s.exe).Enabled
		if err := setStartup(s.exe, on); err != nil {
			s.notify("Start at Login", "Could not change: "+err.Error(), true)
		} else {
			s.log.Printf("start at login = %v (%s)", on, s.exe)
		}
	case CmdOpenFolder:
		windows.ShellExecute(windows.Handle(s.hwnd), utf16Ptr("open"), utf16Ptr(s.cfg.SettingsDir), nil, nil, windows.SW_SHOWNORMAL)
	case CmdQuit:
		postMessage(s.hwnd, wmClose, 0, 0)
	}
	s.updateTray()
}

// ---- rebind capture -----------------------------------------------------
//
// A temporary low-level keyboard hook captures the next key system-wide, so
// no window is needed. It swallows the captured key (down and up) so it does
// not reach the focused application. The hook exists only while capturing.

func (s *Shell) startCapture(a hotkey.Action) {
	if s.capture.active {
		return
	}
	h, _, err := procSetWindowsHookExW.Call(whKeyboardLL, s.hookProc, uintptr(s.hinst), 0)
	if h == 0 {
		s.notify("Rebind failed", "Could not capture the keyboard: "+err.Error(), true)
		return
	}
	s.hook = h
	s.capture.active, s.capture.action = true, a
	s.capture.pressed, s.capture.finishing = 0, false
	s.core.BeginRebind(a)
	procSetTimer.Call(uintptr(s.hwnd), timerCapture, uintptr(captureTimeout/time.Millisecond), 0)
	cur := s.core.Snapshot().Bindings.Get(a)
	s.notify("Rebind "+a.String(), fmt.Sprintf("Press the new key now (currently %s). Esc cancels.", cur), false)
}

func (s *Shell) keyboardProc(code, wp, lp uintptr) uintptr {
	if int32(code) == hcAction && s.capture.active {
		k := hookStruct(lp)
		vk := k.VkCode
		switch uint32(wp) {
		case wmKeyDown, wmSysKeyDown:
			if !s.capture.finishing && !isModifierVK(vk) {
				if s.capture.pressed == 0 {
					s.capture.pressed = vk
					postMessage(s.hwnd, wmCaptured, uintptr(vk), 0)
				}
				return 1
			}
		case wmKeyUp, wmSysKeyUp:
			if vk == s.capture.pressed && vk != 0 {
				postMessage(s.hwnd, wmCaptureKeyUp, uintptr(vk), 0)
				return 1
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, code, wp, lp)
	return r
}

func (s *Shell) onCaptured(vk uint32) {
	if !s.capture.active || s.capture.finishing {
		return
	}
	procKillTimer.Call(uintptr(s.hwnd), timerCapture)
	s.capture.finishing = true
	// Safety net: release the hook even if the key-up never arrives.
	procSetTimer.Call(uintptr(s.hwnd), timerUnhook, 3000, 0)
	a := s.capture.action
	old := s.core.Snapshot().Bindings.Get(a)
	if vk == vkEscape {
		s.core.CancelRebind()
		s.notify("Rebind cancelled", fmt.Sprintf("%s stays on %s.", a, old), false)
		return
	}
	k, ok := keyByVK[vk]
	if !ok {
		s.core.CancelRebind()
		s.notify("Key not supported", fmt.Sprintf("That key (code 0x%02X) can't be bound. %s stays on %s.", vk, a, old), true)
		return
	}
	if err := s.core.CompleteRebind(k); err != nil {
		s.notify("Rebind failed", fmt.Sprintf("%v. %s stays on %s.", err, a, old), true)
		return
	}
	s.notify("Rebound", fmt.Sprintf("%s is now %s.", a, k), false)
}

func (s *Shell) stopCapture() {
	if s.hook != 0 {
		procUnhookWindowsHookEx.Call(s.hook)
		s.hook = 0
	}
	if s.capture.active && !s.capture.finishing && s.core != nil {
		s.core.CancelRebind()
	}
	procKillTimer.Call(uintptr(s.hwnd), timerCapture)
	procKillTimer.Call(uintptr(s.hwnd), timerUnhook)
	s.capture.active, s.capture.finishing, s.capture.pressed = false, false, 0
	s.updateTray()
}
