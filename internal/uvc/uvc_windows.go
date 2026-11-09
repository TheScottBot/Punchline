//go:build windows

package uvc

// Windows backend: DirectShow's IAMCameraControl, which the in-box USB Video
// Class driver (usbvideo.sys) maps onto the UVC Camera Terminal zoom control.
// Pure Go COM; no CGo. Every COM call runs on one dedicated, locked OS thread.

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modOle32    = windows.NewLazySystemDLL("ole32.dll")
	modOleAut32 = windows.NewLazySystemDLL("oleaut32.dll")
	modCfgMgr32 = windows.NewLazySystemDLL("cfgmgr32.dll")

	procCoInitializeEx   = modOle32.NewProc("CoInitializeEx")
	procCoCreateInstance = modOle32.NewProc("CoCreateInstance")
	procVariantClear     = modOleAut32.NewProc("VariantClear")
	procCMLocateDevNode  = modCfgMgr32.NewProc("CM_Locate_DevNodeW")
	procCMGetParent      = modCfgMgr32.NewProc("CM_Get_Parent")
	procCMGetDeviceID    = modCfgMgr32.NewProc("CM_Get_Device_IDW")

	clsidSystemDeviceEnum         = windows.GUID{Data1: 0x62BE5D10, Data2: 0x60EB, Data3: 0x11d0, Data4: [8]byte{0xBD, 0x3B, 0x00, 0xA0, 0xC9, 0x11, 0xCE, 0x86}}
	iidICreateDevEnum             = windows.GUID{Data1: 0x29840822, Data2: 0x5B84, Data3: 0x11D0, Data4: [8]byte{0xBD, 0x3B, 0x00, 0xA0, 0xC9, 0x11, 0xCE, 0x86}}
	clsidVideoInputDeviceCategory = windows.GUID{Data1: 0x860BB310, Data2: 0x5D01, Data3: 0x11d0, Data4: [8]byte{0xBD, 0x3B, 0x00, 0xA0, 0xC9, 0x11, 0xCE, 0x86}}
	iidIPropertyBag               = windows.GUID{Data1: 0x55272A00, Data2: 0x42CB, Data3: 0x11CE, Data4: [8]byte{0x81, 0x35, 0x00, 0xAA, 0x00, 0x4B, 0xB8, 0x51}}
	iidIBaseFilter                = windows.GUID{Data1: 0x56A86895, Data2: 0x0AD4, Data3: 0x11CE, Data4: [8]byte{0xB0, 0x3A, 0x00, 0x20, 0xAF, 0x0B, 0xA7, 0x70}}
	iidIAMCameraControl           = windows.GUID{Data1: 0xC6E13370, Data2: 0x30AC, Data3: 0x11d0, Data4: [8]byte{0xA1, 0x8C, 0x00, 0xA0, 0xC9, 0x11, 0x89, 0x56}}
)

const (
	sOK                 = 0
	sFalse              = 1
	coinitMultithreaded = 0x0
	clsctxInprocServer  = 0x1
	vtBSTR              = 8
	cameraControlZoom   = 3
	cameraControlManual = 0x2
	crSuccess           = 0
	cmLocateNormal      = 0
	maxDeviceIDLen      = 200

	// vtable indices
	vtQueryInterface        = 0
	vtRelease               = 2
	vtCreateClassEnumerator = 3 // ICreateDevEnum
	vtEnumNext              = 3 // IEnumMoniker
	vtBindToObject          = 8 // IMoniker
	vtBindToStorage         = 9 // IMoniker
	vtPropertyBagRead       = 3 // IPropertyBag
	vtCamGetRange           = 3 // IAMCameraControl
	vtCamSet                = 4
	vtCamGet                = 5
)

// HRESULTError reports a failed COM call.
type HRESULTError struct {
	Op string
	HR uint32
}

func (e *HRESULTError) Error() string { return fmt.Sprintf("uvc: %s: HRESULT 0x%08X", e.Op, e.HR) }

func failed(hr uintptr) bool { return int32(uint32(hr)) < 0 }

func hrErr(op string, hr uintptr) error { return &HRESULTError{Op: op, HR: uint32(hr)} }

// comCall invokes method idx of the COM object obj.
//
//go:uintptrescapes
func comCall(obj unsafe.Pointer, idx int, args ...uintptr) uintptr {
	vt := *(*unsafe.Pointer)(obj)
	fn := *(*uintptr)(unsafe.Add(vt, idx*int(unsafe.Sizeof(uintptr(0)))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(obj))
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return r
}

func release(obj unsafe.Pointer) {
	if obj != nil {
		comCall(obj, vtRelease)
	}
}

// variant matches the 64-bit VARIANT layout (24 bytes).
type variant struct {
	vt   uint16
	_    [3]uint16
	val  unsafe.Pointer
	val2 uintptr
}

// comThread runs functions on one OS thread initialised for COM.
type comThread struct {
	calls chan func()
	err   error
}

var (
	threadOnce sync.Once
	thread     *comThread
)

func getThread() *comThread {
	threadOnce.Do(func() {
		thread = &comThread{calls: make(chan func())}
		ready := make(chan struct{})
		go func() {
			runtime.LockOSThread()
			hr, _, _ := procCoInitializeEx.Call(0, coinitMultithreaded)
			if failed(hr) {
				thread.err = hrErr("CoInitializeEx", hr)
			}
			close(ready)
			for f := range thread.calls {
				f()
			}
		}()
		<-ready
	})
	return thread
}

func (t *comThread) do(f func() error) error {
	if t.err != nil {
		return t.err
	}
	done := make(chan error, 1)
	t.calls <- func() { done <- f() }
	return <-done
}

type dshow struct{}

// System returns the DirectShow backend.
func System() Backend { return dshow{} }

func (dshow) Enumerate() ([]Node, error) {
	var nodes []Node
	err := getThread().do(func() error {
		return eachMoniker(func(m unsafe.Pointer) bool {
			name, path := monikerProps(m)
			if path == "" {
				return false // virtual cameras without a device path cannot be UVC
			}
			n := Node{Path: path, Name: name}
			if v, p, ok := parseUSBIDs(path); ok {
				n.VendorID, n.ProductID = v, p
			}
			n.StableID = stableID(n)
			nodes = append(nodes, n)
			return false
		})
	})
	portless(nodes)
	return nodes, err
}

func (dshow) Open(path string) (Control, error) {
	c := &control{}
	err := getThread().do(func() error {
		var filter unsafe.Pointer
		var bindErr error
		found := false
		err := eachMoniker(func(m unsafe.Pointer) bool {
			_, p := monikerProps(m)
			if !strings.EqualFold(p, path) {
				return false
			}
			found = true
			hr := comCall(m, vtBindToObject, 0, 0,
				uintptr(unsafe.Pointer(&iidIBaseFilter)), uintptr(unsafe.Pointer(&filter)))
			if failed(hr) {
				bindErr = hrErr("IMoniker::BindToObject", hr)
			}
			return true
		})
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if bindErr != nil {
			return bindErr
		}
		var cc unsafe.Pointer
		hr := comCall(filter, vtQueryInterface,
			uintptr(unsafe.Pointer(&iidIAMCameraControl)), uintptr(unsafe.Pointer(&cc)))
		if failed(hr) || cc == nil {
			release(filter)
			return fmt.Errorf("%w (IAMCameraControl: 0x%08X)", ErrNoZoom, uint32(hr))
		}
		c.filter, c.cc = filter, cc
		return nil
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}

// eachMoniker calls fn for each video input device moniker until fn
// returns true. Runs on the COM thread.
func eachMoniker(fn func(m unsafe.Pointer) bool) error {
	var devEnum unsafe.Pointer
	hr, _, _ := procCoCreateInstance.Call(
		uintptr(unsafe.Pointer(&clsidSystemDeviceEnum)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidICreateDevEnum)), uintptr(unsafe.Pointer(&devEnum)))
	if failed(hr) {
		return hrErr("CoCreateInstance(SystemDeviceEnum)", hr)
	}
	defer release(devEnum)
	var enum unsafe.Pointer
	hr = comCall(devEnum, vtCreateClassEnumerator,
		uintptr(unsafe.Pointer(&clsidVideoInputDeviceCategory)), uintptr(unsafe.Pointer(&enum)), 0)
	if failed(hr) {
		return hrErr("CreateClassEnumerator", hr)
	}
	if hr == sFalse || enum == nil {
		return nil // category empty
	}
	defer release(enum)
	for {
		var m unsafe.Pointer
		var fetched uint32
		hr = comCall(enum, vtEnumNext, 1, uintptr(unsafe.Pointer(&m)), uintptr(unsafe.Pointer(&fetched)))
		if hr != sOK || fetched == 0 || m == nil {
			return nil
		}
		stop := fn(m)
		release(m)
		if stop {
			return nil
		}
	}
}

func monikerProps(m unsafe.Pointer) (name, path string) {
	var bag unsafe.Pointer
	hr := comCall(m, vtBindToStorage, 0, 0,
		uintptr(unsafe.Pointer(&iidIPropertyBag)), uintptr(unsafe.Pointer(&bag)))
	if failed(hr) || bag == nil {
		return "", ""
	}
	defer release(bag)
	return bagString(bag, "FriendlyName"), bagString(bag, "DevicePath")
}

func bagString(bag unsafe.Pointer, prop string) string {
	p, err := windows.UTF16PtrFromString(prop)
	if err != nil {
		return ""
	}
	v := new(variant)
	hr := comCall(bag, vtPropertyBagRead, uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(v)), 0)
	defer procVariantClear.Call(uintptr(unsafe.Pointer(v)))
	if failed(hr) || v.vt != vtBSTR || v.val == nil {
		return ""
	}
	return windows.UTF16PtrToString((*uint16)(v.val))
}

// stableID derives a reconnect-stable identifier. For composite USB devices
// the DirectShow path names an interface (MI_xx) whose instance ID depends on
// the USB port; the parent device's instance ID carries the serial number
// when the device reports one.
func stableID(n Node) string {
	inst := instanceIDFromPath(n.Path)
	if n.VendorID == 0 && n.ProductID == 0 {
		return "path:" + strings.ToLower(inst)
	}
	id := inst
	if strings.Contains(strings.ToLower(inst), "&mi_") {
		if parent, ok := parentInstanceID(inst); ok {
			id = parent
		}
	}
	parts := strings.Split(id, `\`)
	last := parts[len(parts)-1]
	if last != "" && !strings.Contains(last, "&") {
		return n.USBID() + ":" + last // genuine serial number
	}
	return n.USBID() + "@" + strings.ToLower(inst)
}

// instanceIDFromPath converts \\?\usb#vid..#inst#{guid}\global into
// usb\vid..\inst.
func instanceIDFromPath(path string) string {
	s := strings.TrimPrefix(path, `\\?\`)
	if i := strings.Index(s, "#{"); i >= 0 {
		s = s[:i]
	}
	return strings.ReplaceAll(s, "#", `\`)
}

func parentInstanceID(inst string) (string, bool) {
	p, err := windows.UTF16PtrFromString(inst)
	if err != nil {
		return "", false
	}
	var dev, parent uint32
	if r, _, _ := procCMLocateDevNode.Call(uintptr(unsafe.Pointer(&dev)), uintptr(unsafe.Pointer(p)), cmLocateNormal); r != crSuccess {
		return "", false
	}
	if r, _, _ := procCMGetParent.Call(uintptr(unsafe.Pointer(&parent)), uintptr(dev), 0); r != crSuccess {
		return "", false
	}
	buf := make([]uint16, maxDeviceIDLen+1)
	if r, _, _ := procCMGetDeviceID.Call(uintptr(parent), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0); r != crSuccess {
		return "", false
	}
	return windows.UTF16ToString(buf), true
}

type control struct {
	mu     sync.Mutex
	filter unsafe.Pointer
	cc     unsafe.Pointer
}

func (c *control) ZoomRange() (Range, error) {
	var r Range
	err := c.call(func(cc unsafe.Pointer) error {
		var lo, hi, step, def, caps int32
		hr := comCall(cc, vtCamGetRange, cameraControlZoom,
			uintptr(unsafe.Pointer(&lo)), uintptr(unsafe.Pointer(&hi)), uintptr(unsafe.Pointer(&step)),
			uintptr(unsafe.Pointer(&def)), uintptr(unsafe.Pointer(&caps)))
		if failed(hr) {
			return fmt.Errorf("%w: %v", ErrNoZoom, hrErr("IAMCameraControl::GetRange", hr))
		}
		r = Range{Min: lo, Max: hi, Step: step, Default: def}
		return nil
	})
	return r, err
}

func (c *control) Zoom() (int32, error) {
	var v int32
	err := c.call(func(cc unsafe.Pointer) error {
		var flags int32
		hr := comCall(cc, vtCamGet, cameraControlZoom, uintptr(unsafe.Pointer(&v)), uintptr(unsafe.Pointer(&flags)))
		if failed(hr) {
			return hrErr("IAMCameraControl::Get", hr)
		}
		return nil
	})
	return v, err
}

func (c *control) SetZoom(v int32) error {
	return c.call(func(cc unsafe.Pointer) error {
		hr := comCall(cc, vtCamSet, cameraControlZoom, uintptr(int64(v)), cameraControlManual)
		if failed(hr) {
			return hrErr("IAMCameraControl::Set", hr)
		}
		return nil
	})
}

func (c *control) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cc == nil {
		return nil
	}
	cc, f := c.cc, c.filter
	c.cc, c.filter = nil, nil
	return getThread().do(func() error {
		release(cc)
		release(f)
		return nil
	})
}

func (c *control) call(f func(cc unsafe.Pointer) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cc == nil {
		return ErrClosed
	}
	cc := c.cc
	return getThread().do(func() error { return f(cc) })
}
