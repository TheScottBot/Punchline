//go:build darwin

package desktop

// macOS menu bar shell. The native half (shell_darwin.m) owns the Cocoa
// objects; this file holds the behaviour and mirrors shell_windows.go.
//
// Hotkeys use Carbon RegisterEventHotKey: no Input Monitoring or
// Accessibility permission is needed, the key is consumed (it does not also
// reach Teams), and a key another app holds exclusively is reported as
// unavailable. Rebinding captures the next key in a temporary panel (spec
// DCU-011 allows a temporary capture prompt), which also needs no
// permission. macOS has no key codes beyond F20, so F21-F24 cannot be bound.

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa -framework Carbon -framework IOKit
#include <stdlib.h>
#include "shell_darwin.h"
*/
import "C"

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/cgo"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/thescottbot/punchline/internal/app"
	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/icon"
	"github.com/thescottbot/punchline/internal/midi"
	"github.com/thescottbot/punchline/internal/product"
	"github.com/thescottbot/punchline/internal/settings"
)

// Cocoa requires the main thread; main's goroutine starts on it.
func init() { runtime.LockOSThread() }

const (
	resultLinger         = 1500 * time.Millisecond
	deviceDebounce       = 700 * time.Millisecond
	iconPoints           = 18
	iconScale            = 2
	lockFileName         = product.ExeName + ".lock"
	osStatusHotKeyExists = -9878 // eventHotKeyExistsErr
)

// Shell is the macOS desktop integration.
type Shell struct {
	cfg  Config
	log  *log.Logger
	exe  string
	lock *os.File
	hk   *hotkeyTable
	reg  *macRegistrar
	rb   *rebinder // set in Run
	core *app.Core

	midi      *midi.Client
	midiState MIDIState // main thread

	cmds   []Command
	notice string // shown in the menu until the next menu action

	usbGen int

	iconSet   bool
	lastStyle icon.Style
	lastTip   string

	timers       map[int]func()
	nextTimer    int
	shutdownDone bool
}

// shell is the single instance the native callbacks reach.
var shell *Shell

func lockPath(dir string) string { return filepath.Join(dir, lockFileName) }

// tryLock takes the single-instance lock and records this process's PID in
// it, or reports that another instance holds it.
func tryLock(dir string) (*os.File, bool, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, false, err
	}
	f, err := os.OpenFile(lockPath(dir), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, true, nil
		}
		return nil, false, err
	}
	f.Truncate(0)
	f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	return f, false, nil
}

// signalRunning sends sig to the instance holding the lock in dir.
// Signals rather than distributed notifications: they need no window
// server session and work from any shell or launchd.
func signalRunning(dir string, sig syscall.Signal) bool {
	b, err := os.ReadFile(lockPath(dir))
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil || pid <= 0 || pid == os.Getpid() {
		return false
	}
	return syscall.Kill(pid, sig) == nil
}

// New claims the single-instance lock and sets up the menu bar item. A
// second launch opens the running instance's menu and returns
// ErrAlreadyRunning.
func New(cfg Config) (*Shell, error) {
	cfg.defaults()
	f, held, err := tryLock(cfg.SettingsDir)
	if err != nil {
		return nil, fmt.Errorf("single-instance lock: %w", err)
	}
	if held {
		signalRunning(cfg.SettingsDir, syscall.SIGUSR1) // opens its menu
		return nil, ErrAlreadyRunning
	}
	s := &Shell{cfg: cfg, log: cfg.Logger, lock: f, timers: map[int]func(){}}
	if exe, err := os.Executable(); err == nil {
		s.exe = exe
	}
	s.hk = newHotkeyTable(carbonHotkeys{}, 1<<31)
	s.reg = &macRegistrar{s: s}
	shell = s
	C.pl_app_init()
	return s, nil
}

// Registrar returns the OS hotkey registrar for the core.
func (s *Shell) Registrar() app.Registrar { return s.reg }

// Changed may be called from any goroutine when core state changes.
func (s *Shell) Changed() {
	s.onMain(func() {
		if s.core != nil {
			s.core.SyncHotkeys() // e.g. the selected camera gained or lost a halfway level
		}
		s.updateIcon()
	}, false)
}

// Run shows the menu bar item, starts the core and runs the Cocoa event loop
// until Quit.
func (s *Shell) Run(core *app.Core, notices []string) error {
	s.core = core
	s.rb = &rebinder{core: core, ui: panelUI{s}, log: s.log}
	s.updateIcon()
	C.pl_watch_usb()
	if err := core.Start(); err != nil {
		s.log.Printf("notice: hotkey problem: %v", err) // the menu shows it too
	}
	s.startMIDI()
	if len(notices) > 0 {
		s.setNotice("Settings: " + strings.Join(notices, "; "))
	}
	sig := make(chan os.Signal, 4)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGUSR1)
	go func() {
		for v := range sig {
			if v == syscall.SIGUSR1 {
				s.onMain(func() { C.pl_open_menu() }, false)
				continue
			}
			s.log.Printf("signal %v", v)
			s.onMain(s.quit, false)
			return
		}
	}()
	s.log.Printf("running; exe %s", s.exe)
	C.pl_app_run()
	signal.Stop(sig)
	s.shutdown()
	s.release()
	return nil
}

// RequestQuit asks a running instance to quit. It reports whether one was
// running.
func RequestQuit() bool {
	dir, err := settings.Dir()
	if err != nil {
		return false
	}
	f, held, err := tryLock(dir)
	if err != nil {
		return false
	}
	if !held {
		f.Close()
		return false
	}
	return signalRunning(dir, syscall.SIGTERM)
}

func (s *Shell) quit() {
	s.log.Printf("quit requested")
	s.shutdown()
	C.pl_app_stop()
}

func (s *Shell) shutdown() {
	if s.shutdownDone {
		return
	}
	s.shutdownDone = true
	s.stopCapture()
	if s.midi != nil {
		s.midi.Close()
		s.midi = nil
	}
	if s.core != nil {
		s.core.Shutdown()
	}
}

func (s *Shell) release() {
	if s.lock != nil {
		s.lock.Close()
		s.lock = nil
	}
}

// ---- main-thread plumbing -----------------------------------------------

func (s *Shell) onMain(f func(), wait bool) {
	h := cgo.NewHandle(f)
	w := C.int(0)
	if wait {
		w = 1
	}
	C.pl_dispatch_main(C.uintptr_t(h), w)
}

// after runs f on the main thread after d. Call on the main thread.
func (s *Shell) after(d time.Duration, f func()) {
	s.nextTimer++
	s.timers[s.nextTimer] = f
	C.pl_after(C.double(d.Seconds()), C.int(s.nextTimer))
}

// guard logs a panic from a native callback instead of crashing the app.
func guard(where string) {
	if r := recover(); r != nil && shell != nil {
		shell.log.Printf("panic in %s: %v\n%s", where, r, debug.Stack())
	}
}

//export goMainCall
func goMainCall(h C.uintptr_t) {
	defer guard("main-thread call")
	hd := cgo.Handle(h)
	f := hd.Value().(func())
	hd.Delete()
	f()
}

//export goTimer
func goTimer(token C.int) {
	defer guard("timer")
	if shell == nil {
		return
	}
	f, ok := shell.timers[int(token)]
	delete(shell.timers, int(token))
	if ok {
		f()
	}
}

//export goTerminate
func goTerminate() {
	defer guard("terminate")
	if shell != nil {
		shell.log.Printf("session ending")
		shell.shutdown()
	}
}

//export goUSBChanged
func goUSBChanged() {
	defer guard("usb change")
	s := shell
	if s == nil || s.core == nil {
		return
	}
	s.usbGen++
	g := s.usbGen
	s.after(deviceDebounce, func() {
		if g == s.usbGen && s.core != nil {
			s.core.TriggerRescan()
		}
	})
}

// ---- menu bar item ------------------------------------------------------

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

func (s *Shell) updateIcon() {
	sn := s.snapshot()
	st, tip := styleFor(sn), Tooltip(sn)
	if s.iconSet && st == s.lastStyle && tip == s.lastTip {
		return
	}
	px := iconPoints * iconScale
	rgba := toRGBAPremultiplied(icon.Render(px, st))
	ctip := C.CString(tip)
	defer C.free(unsafe.Pointer(ctip))
	C.pl_set_icon(unsafe.Pointer(&rgba[0]), C.int(px), C.double(iconPoints), ctip)
	s.iconSet, s.lastStyle, s.lastTip = true, st, tip
}

// toRGBAPremultiplied converts icon.Render's straight-alpha BGRA to the
// premultiplied RGBA an NSBitmapImageRep expects by default.
func toRGBAPremultiplied(bgra []byte) []byte {
	out := make([]byte, len(bgra))
	for i := 0; i+3 < len(bgra); i += 4 {
		a := uint32(bgra[i+3])
		out[i+0] = byte(uint32(bgra[i+2]) * a / 255)
		out[i+1] = byte(uint32(bgra[i+1]) * a / 255)
		out[i+2] = byte(uint32(bgra[i+0]) * a / 255)
		out[i+3] = byte(a)
	}
	return out
}

func (s *Shell) setNotice(n string) {
	s.notice = n
	s.log.Printf("notice: %s", n)
}

//export goBuildMenu
func goBuildMenu(menu unsafe.Pointer) {
	defer guard("menu")
	s := shell
	if s == nil || s.core == nil {
		return
	}
	items := BuildMenu(s.core.Snapshot(), startupState(s.exe), PermissionState{}, s.midiState)
	if s.notice != "" && len(items) >= 2 {
		items = append(items[:2], append([]Item{label(truncate(s.notice, 90))}, items[2:]...)...)
	}
	s.cmds = s.cmds[:0]
	s.addItems(menu, items)
}

func (s *Shell) addItems(menu unsafe.Pointer, items []Item) {
	b := func(v bool) C.int {
		if v {
			return 1
		}
		return 0
	}
	for _, it := range items {
		tag := 0
		if it.Kind == KindCommand {
			s.cmds = append(s.cmds, it.Cmd)
			tag = len(s.cmds)
		}
		text := C.CString(it.Text)
		sub := C.pl_menu_add(menu, C.int(it.Kind), text, b(it.Checked), b(it.Disabled), C.int(tag))
		C.free(unsafe.Pointer(text))
		if it.Kind == KindSubmenu && sub != nil {
			s.addItems(sub, it.Children)
		}
	}
}

//export goMenuAction
func goMenuAction(tag C.int) {
	defer guard("menu action")
	s := shell
	if s == nil || s.core == nil || int(tag) < 1 || int(tag) > len(s.cmds) {
		return
	}
	s.notice = ""
	s.run(s.cmds[tag-1])
}

func (s *Shell) run(c Command) {
	switch c.Kind {
	case CmdToggleHotkeys:
		if err := s.core.SetEnabled(!s.core.Snapshot().Enabled); err != nil {
			s.setNotice("Hotkey problem: " + err.Error())
		}
	case CmdRebind:
		s.rb.Start(c.Action)
	case CmdSelectCamera:
		s.core.SelectCamera(c.Camera)
	case CmdToggleStartup:
		on := !startupState(s.exe).Enabled
		if err := setStartup(s.exe, on); err != nil {
			s.setNotice("Start at Login: could not change: " + err.Error())
		} else {
			s.log.Printf("start at login = %v (%s)", on, s.exe)
		}
	case CmdOpenFolder:
		p := C.CString(s.cfg.SettingsDir)
		C.pl_open_path(p)
		C.free(unsafe.Pointer(p))
	case CmdQuit:
		s.quit()
		return
	}
	s.updateIcon()
}

// ---- rebind capture -----------------------------------------------------
//
// The shared rebinder (rebind.go) runs the flow; the native half is a
// temporary panel that becomes key and swallows the next key down.

type panelUI struct{ s *Shell }

func (u panelUI) Prompt(title, text string) error {
	t, x := C.CString(title), C.CString(text)
	C.pl_capture_show(t, x)
	C.free(unsafe.Pointer(t))
	C.free(unsafe.Pointer(x))
	return nil
}

// Result shows the outcome in the panel, then closes it unless another
// rebind has started meanwhile.
func (u panelUI) Result(title, text string, warn bool) {
	x := C.CString(text)
	C.pl_capture_text(x)
	C.free(unsafe.Pointer(x))
	g := u.s.rb.gen
	u.s.after(resultLinger, func() {
		if !u.s.rb.Active() && u.s.rb.gen == g {
			C.pl_capture_close()
		}
	})
	u.s.updateIcon()
}

func (u panelUI) After(d time.Duration, f func()) { u.s.after(d, f) }

//export goCaptureKey
func goCaptureKey(code C.ushort) {
	defer guard("capture")
	s := shell
	if s == nil || s.rb == nil {
		return
	}
	c := uint16(code)
	if c == kvkEscape {
		s.rb.Captured(hotkey.Cancel, true, uint32(c))
		return
	}
	k, ok := keyByCode[c]
	s.rb.Captured(k, ok, uint32(c))
}

//export goCaptureCancel
func goCaptureCancel() {
	defer guard("capture cancel")
	if s := shell; s != nil && s.rb != nil && s.rb.Abort("panel closed or lost focus") {
		C.pl_capture_close()
	}
}

func (s *Shell) stopCapture() {
	if s.rb != nil {
		s.rb.Abort("shutdown")
	}
	C.pl_capture_close()
}

// ---- hotkeys ------------------------------------------------------------

// carbonHotkeys registers Carbon hot keys (main thread only).
type carbonHotkeys struct{}

func (carbonHotkeys) register(k hotkey.Key, id uint32) (any, error) {
	code, ok := codeByKey[k]
	if !ok {
		return nil, fmt.Errorf("%s is not available on macOS: %w", k, hotkey.ErrUnsupportedKey)
	}
	var ref unsafe.Pointer
	switch st := C.pl_hotkey_register(C.int(code), C.uint32_t(id), &ref); {
	case st == osStatusHotKeyExists:
		return nil, fmt.Errorf("%s is held by another application: %w", k, hotkey.ErrUnavailable)
	case st != 0:
		return nil, fmt.Errorf("%s: RegisterEventHotKey failed (OSStatus %d): %w", k, int(st), hotkey.ErrUnavailable)
	}
	return ref, nil
}

func (carbonHotkeys) unregister(h any) {
	if ref, ok := h.(unsafe.Pointer); ok {
		C.pl_hotkey_unregister(ref)
	}
}

// macRegistrar is the core's Registrar: the shared table, always on the main
// thread.
type macRegistrar struct{ s *Shell }

func (r *macRegistrar) Register(a hotkey.Action, k hotkey.Key) error {
	var err error
	r.s.onMain(func() { err = r.s.hk.Register(a, k) }, true)
	return err
}

func (r *macRegistrar) Unregister(a hotkey.Action) {
	r.s.onMain(func() { r.s.hk.Unregister(a) }, true)
}

//export goHotkey
func goHotkey(id C.uint32_t) {
	defer guard("hotkey")
	if s := shell; s != nil && s.core != nil && s.rb != nil {
		dispatchHotkey(s.hk, s.rb, s.core, uint32(id))
	}
}

// ---- MIDI ---------------------------------------------------------------

// startMIDI listens to every MIDI input (CoreMIDI is multi-client, so RØDE
// Central or a DAW keeps working). Opened on the main thread so source
// changes arrive on its run loop. A failure is shown in the menu; keys keep
// working.
func (s *Shell) startMIDI() {
	s.midiState.Supported = true
	router := midiRouter{
		core:   s.core,
		onUI:   func(f func()) { s.onMain(f, false) },
		rebind: func(k hotkey.Key) { s.rb.Captured(k, true, 0) },
	}
	c, err := midi.Open(router.Event, func(ss []midi.Source) {
		s.onMain(func() { s.setMIDISources(ss) }, false)
	})
	if err != nil {
		s.midiState.Err = err.Error()
		s.log.Printf("MIDI: %v", err)
		return
	}
	s.midi = c
	s.rb.midi = true
	s.setMIDISources(c.Sources())
}

func (s *Shell) setMIDISources(ss []midi.Source) {
	names := make([]string, 0, len(ss))
	for _, x := range ss {
		names = append(names, x.Name)
	}
	s.midiState.Sources = names
	s.log.Printf("MIDI inputs: %v", names)
}
