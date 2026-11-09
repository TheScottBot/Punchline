//go:build windows

package desktop

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modUser32   = windows.NewLazySystemDLL("user32.dll")
	modShell32  = windows.NewLazySystemDLL("shell32.dll")
	modGdi32    = windows.NewLazySystemDLL("gdi32.dll")
	modKernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW          = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW           = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW            = modUser32.NewProc("DefWindowProcW")
	procDestroyWindow             = modUser32.NewProc("DestroyWindow")
	procGetMessageW               = modUser32.NewProc("GetMessageW")
	procTranslateMessage          = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW          = modUser32.NewProc("DispatchMessageW")
	procPostMessageW              = modUser32.NewProc("PostMessageW")
	procPostQuitMessage           = modUser32.NewProc("PostQuitMessage")
	procRegisterHotKey            = modUser32.NewProc("RegisterHotKey")
	procUnregisterHotKey          = modUser32.NewProc("UnregisterHotKey")
	procCreatePopupMenu           = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW               = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu            = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu               = modUser32.NewProc("DestroyMenu")
	procSetForegroundWindow       = modUser32.NewProc("SetForegroundWindow")
	procSetWindowsHookExW         = modUser32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx       = modUser32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx            = modUser32.NewProc("CallNextHookEx")
	procSetTimer                  = modUser32.NewProc("SetTimer")
	procKillTimer                 = modUser32.NewProc("KillTimer")
	procRegisterWindowMessageW    = modUser32.NewProc("RegisterWindowMessageW")
	procFindWindowW               = modUser32.NewProc("FindWindowW")
	procCreateIconIndirect        = modUser32.NewProc("CreateIconIndirect")
	procDestroyIcon               = modUser32.NewProc("DestroyIcon")
	procGetSystemMetrics          = modUser32.NewProc("GetSystemMetrics")
	procGetDC                     = modUser32.NewProc("GetDC")
	procReleaseDC                 = modUser32.NewProc("ReleaseDC")
	procGetCursorPos              = modUser32.NewProc("GetCursorPos")
	procSetProcessDpiAwarenessCtx = modUser32.NewProc("SetProcessDpiAwarenessContext")
	procShellNotifyIconW          = modShell32.NewProc("Shell_NotifyIconW")
	procCreateDIBSection          = modGdi32.NewProc("CreateDIBSection")
	procCreateBitmap              = modGdi32.NewProc("CreateBitmap")
	procDeleteObject              = modGdi32.NewProc("DeleteObject")
	procGetModuleHandleW          = modKernel32.NewProc("GetModuleHandleW")
)

const (
	wmNull            = 0x0000
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
	wmContextMenu     = 0x007B
	wmKeyDown         = 0x0100
	wmKeyUp           = 0x0101
	wmSysKeyDown      = 0x0104
	wmSysKeyUp        = 0x0105
	wmTimer           = 0x0113
	wmDeviceChange    = 0x0219
	wmHotkey          = 0x0312
	wmApp             = 0x8000

	wmTrayCallback   = wmApp + 1
	wmStateChanged   = wmApp + 2
	wmCaptured       = wmApp + 3
	wmCaptureKeyUp   = wmApp + 4
	wmAlreadyRunning = wmApp + 5

	ninSelect    = 0x0400
	ninKeySelect = 0x0401

	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nimSetVersion = 4
	nifMessage    = 0x01
	nifIcon       = 0x02
	nifTip        = 0x04
	nifInfo       = 0x10
	nifShowTip    = 0x80
	niifInfo      = 0x01
	niifWarning   = 0x02
	niifNoSound   = 0x10
	notifyIconV4  = 4

	mfString    = 0x0000
	mfGrayed    = 0x0001
	mfChecked   = 0x0008
	mfPopup     = 0x0010
	mfSeparator = 0x0800

	tpmRightButton = 0x0002
	tpmNoNotify    = 0x0080
	tpmReturnCmd   = 0x0100

	wsPopup        = 0x80000000
	wsExToolWindow = 0x00000080

	modNoRepeat = 0x4000

	whKeyboardLL = 13
	hcAction     = 0

	dbtDevNodesChanged      = 0x0007
	dbtDeviceArrival        = 0x8000
	dbtDeviceRemoveComplete = 0x8004

	smCxSmIcon = 49

	errorHotkeyAlreadyRegistered = 1409

	vkEscape = 0x1B
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type point struct{ X, Y int32 }

type msg struct {
	Hwnd    windows.HWND
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	Private uint32
}

type notifyIconData struct {
	CbSize          uint32
	Wnd             windows.HWND
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	Icon            windows.Handle
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        windows.GUID
	BalloonIcon     windows.Handle
}

type kbdllHookStruct struct {
	VkCode    uint32
	ScanCode  uint32
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  windows.Handle
	HbmColor windows.Handle
}

func utf16Ptr(s string) *uint16 {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		p, _ = windows.UTF16PtrFromString("?")
	}
	return p
}

// copyUTF16 copies s into a fixed buffer, truncating and NUL-terminating.
func copyUTF16(dst []uint16, s string) {
	u, _ := windows.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)]
		u[len(u)-1] = 0
	}
	copy(dst, u)
}

func postMessage(h windows.HWND, m uint32, w, l uintptr) {
	procPostMessageW.Call(uintptr(h), uintptr(m), w, l)
}

func loword(v uintptr) uint16 { return uint16(v & 0xFFFF) }
func hiword(v uintptr) uint16 { return uint16((v >> 16) & 0xFFFF) }

// hookStruct reads the KBDLLHOOKSTRUCT a low-level keyboard hook receives.
func hookStruct(lparam uintptr) *kbdllHookStruct {
	return *(**kbdllHookStruct)(unsafe.Pointer(&lparam))
}

// createIcon builds an HICON from top-down BGRA pixels with straight alpha.
func createIcon(size int, px []byte) (windows.Handle, error) {
	bi := bitmapInfo{Header: bitmapInfoHeader{
		Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(size), Height: -int32(size),
		Planes: 1, BitCount: 32,
	}}
	hdc, _, _ := procGetDC.Call(0)
	var bits unsafe.Pointer
	hbm, _, err := procCreateDIBSection.Call(hdc, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	procReleaseDC.Call(0, hdc)
	if hbm == 0 || bits == nil {
		return 0, err
	}
	defer procDeleteObject.Call(hbm)
	copy(unsafe.Slice((*byte)(bits), len(px)), px)
	mask, _, err := procCreateBitmap.Call(uintptr(size), uintptr(size), 1, 1, 0)
	if mask == 0 {
		return 0, err
	}
	defer procDeleteObject.Call(mask)
	ii := iconInfo{FIcon: 1, HbmMask: windows.Handle(mask), HbmColor: windows.Handle(hbm)}
	h, _, err := procCreateIconIndirect.Call(uintptr(unsafe.Pointer(&ii)))
	if h == 0 {
		return 0, err
	}
	return windows.Handle(h), nil
}
