//go:build darwin

package uvc

// macOS backend: UVC class requests sent on the device's default control
// pipe through the IOKit USB device user client (IOUSBLib), as nf/logicam
// does. Control requests need neither USBDeviceOpen nor an interface claim,
// so they work while another application (Teams, Discord) streams video.
//
// The Video Control interface number and camera terminal ID come from the
// configuration descriptor, parsed in desc.go.

/*
#cgo LDFLAGS: -framework IOKit -framework CoreFoundation
#include <stdlib.h>
#include <string.h>
#include <CoreFoundation/CoreFoundation.h>
#include <IOKit/IOKitLib.h>
#include <IOKit/IOCFPlugIn.h>
#include <IOKit/usb/IOUSBLib.h>

typedef struct {
	uint64_t entry;
	uint16_t vid, pid;
	uint32_t location;
	char name[128];
	char serial[128];
} pl_node;

typedef IOUSBDeviceInterface182 **pl_dev;

static int pl_num(io_service_t s, const char *key, int64_t *out) {
	CFStringRef k = CFStringCreateWithCString(NULL, key, kCFStringEncodingUTF8);
	CFTypeRef v = IORegistryEntryCreateCFProperty(s, k, kCFAllocatorDefault, 0);
	CFRelease(k);
	if (!v) return 0;
	int ok = CFGetTypeID(v) == CFNumberGetTypeID() && CFNumberGetValue((CFNumberRef)v, kCFNumberSInt64Type, out);
	CFRelease(v);
	return ok;
}

static void pl_str(io_service_t s, const char *key, char *buf, size_t n) {
	buf[0] = 0;
	CFStringRef k = CFStringCreateWithCString(NULL, key, kCFStringEncodingUTF8);
	CFTypeRef v = IORegistryEntryCreateCFProperty(s, k, kCFAllocatorDefault, 0);
	CFRelease(k);
	if (!v) return;
	if (CFGetTypeID(v) == CFStringGetTypeID()) CFStringGetCString((CFStringRef)v, buf, n, kCFStringEncodingUTF8);
	CFRelease(v);
}

// pl_has_vc reports whether a USB device has a Video Control interface,
// from its interface children's registry properties (no device I/O).
static int pl_has_vc(io_service_t dev) {
	io_iterator_t it;
	if (IORegistryEntryGetChildIterator(dev, kIOServicePlane, &it) != KERN_SUCCESS) return 0;
	int found = 0;
	io_service_t c;
	while (!found && (c = IOIteratorNext(it))) {
		int64_t cls = -1, sub = -1;
		if (pl_num(c, "bInterfaceClass", &cls) && pl_num(c, "bInterfaceSubClass", &sub) && cls == 0x0e && sub == 0x01)
			found = 1;
		IOObjectRelease(c);
	}
	IOObjectRelease(it);
	return found;
}

// pl_enumerate fills out with USB devices that have a Video Control
// interface. Returns a kern_return_t.
static int pl_enumerate(pl_node *out, int max, int *count) {
	*count = 0;
	io_iterator_t it;
	kern_return_t kr = IOServiceGetMatchingServices(kIOMainPortDefault, IOServiceMatching("IOUSBHostDevice"), &it);
	if (kr != KERN_SUCCESS) return kr;
	io_service_t s;
	while ((s = IOIteratorNext(it))) {
		if (*count < max && pl_has_vc(s)) {
			pl_node *n = &out[*count];
			memset(n, 0, sizeof *n);
			int64_t v = 0;
			IORegistryEntryGetRegistryEntryID(s, &n->entry);
			if (pl_num(s, "idVendor", &v)) n->vid = (uint16_t)v;
			if (pl_num(s, "idProduct", &v)) n->pid = (uint16_t)v;
			if (pl_num(s, "locationID", &v)) n->location = (uint32_t)v;
			pl_str(s, "USB Product Name", n->name, sizeof n->name);
			pl_str(s, "USB Serial Number", n->serial, sizeof n->serial);
			(*count)++;
		}
		IOObjectRelease(s);
	}
	IOObjectRelease(it);
	return KERN_SUCCESS;
}

static int pl_open(uint64_t entry, pl_dev *out) {
	*out = NULL;
	io_service_t s = IOServiceGetMatchingService(kIOMainPortDefault, IORegistryEntryIDMatching(entry));
	if (!s) return kIOReturnNoDevice;
	IOCFPlugInInterface **plug = NULL;
	SInt32 score;
	kern_return_t kr = IOCreatePlugInInterfaceForService(s, kIOUSBDeviceUserClientTypeID, kIOCFPlugInInterfaceID, &plug, &score);
	IOObjectRelease(s);
	if (kr != KERN_SUCCESS) return kr;
	if (!plug) return kIOReturnError;
	pl_dev dev = NULL;
	HRESULT hr = (*plug)->QueryInterface(plug, CFUUIDGetUUIDBytes(kIOUSBDeviceInterfaceID182), (LPVOID *)&dev);
	IODestroyPlugInInterface(plug);
	if (hr || !dev) return kIOReturnUnsupported;
	*out = dev;
	return KERN_SUCCESS;
}

// pl_config copies configuration descriptor 0 into buf. *len is the buffer
// size on entry and the descriptor's total length on return.
static int pl_config(pl_dev d, void *buf, int *len) {
	IOUSBConfigurationDescriptorPtr c = NULL;
	IOReturn r = (*d)->GetConfigurationDescriptorPtr(d, 0, &c);
	if (r != kIOReturnSuccess) return r;
	if (!c) return kIOReturnError;
	int total = USBToHostWord(c->wTotalLength);
	int n = total < *len ? total : *len;
	memcpy(buf, c, n);
	*len = n;
	return kIOReturnSuccess;
}

static int pl_request(pl_dev d, uint8_t type, uint8_t req, uint16_t value, uint16_t index, void *buf, uint16_t len, uint32_t *done) {
	IOUSBDevRequestTO r;
	memset(&r, 0, sizeof r);
	r.bmRequestType = type;
	r.bRequest = req;
	r.wValue = value;
	r.wIndex = index;
	r.wLength = len;
	r.pData = buf;
	r.noDataTimeout = 1000;
	r.completionTimeout = 1000;
	IOReturn kr = (*d)->DeviceRequestTO(d, &r);
	*done = r.wLenDone;
	return kr;
}

static void pl_close(pl_dev d) {
	if (d) (*d)->Release(d);
}
*/
import "C"

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unsafe"
)

// IOReturnError reports a failed IOKit call.
type IOReturnError struct {
	Op   string
	Code uint32
}

var ioReturnNames = map[uint32]string{
	0xe00002bc: "kIOReturnError",
	0xe00002c0: "kIOReturnNoDevice",
	0xe00002c2: "kIOReturnBadArgument",
	0xe00002c5: "kIOReturnExclusiveAccess",
	0xe00002c7: "kIOReturnUnsupported",
	0xe00002d5: "kIOReturnNotOpen",
	0xe00002d6: "kIOReturnTimeout",
	0xe00002eb: "kIOReturnAborted",
	0xe00002ed: "kIOReturnNotResponding",
	0xe000404f: "kIOUSBPipeStalled",
	0xe0004051: "kIOUSBTransactionTimeout",
}

func (e *IOReturnError) Error() string {
	if n, ok := ioReturnNames[e.Code]; ok {
		return fmt.Sprintf("uvc: %s: %s (0x%08x)", e.Op, n, e.Code)
	}
	return fmt.Sprintf("uvc: %s: IOReturn 0x%08x", e.Op, e.Code)
}

func ioErr(op string, kr C.int) error {
	if kr == 0 {
		return nil
	}
	return &IOReturnError{Op: op, Code: uint32(kr)}
}

const (
	maxNodes        = 64
	ioReturnAborted = 0xe00002eb
)

type iokit struct{}

// System returns the IOKit backend.
func System() Backend { return iokit{} }

// Node paths are "iokit:<registry entry ID hex>:<VC interface number>". The
// entry ID changes when the device is re-attached; StableID does not.

func (iokit) Enumerate() ([]Node, error) {
	var raw [maxNodes]C.pl_node
	var count C.int
	if err := ioErr("IOServiceGetMatchingServices", C.pl_enumerate(&raw[0], maxNodes, &count)); err != nil {
		return nil, err
	}
	var nodes []Node
	for i := 0; i < int(count); i++ {
		r := &raw[i]
		base := Node{
			Name:      C.GoString(&r.name[0]),
			VendorID:  uint16(r.vid),
			ProductID: uint16(r.pid),
		}
		if base.Name == "" {
			base.Name = "USB camera " + base.USBID()
		}
		if s := strings.TrimSpace(C.GoString(&r.serial[0])); s != "" {
			base.StableID = base.USBID() + ":" + s
		} else {
			base.StableID = fmt.Sprintf("%s@loc%08x", base.USBID(), uint32(r.location))
		}
		entry := uint64(r.entry)
		fns, err := videoFunctions(entry)
		if err != nil || len(fns) == 0 {
			// List it anyway so diagnostics show it; Open reports the error.
			n := base
			n.Path = fmt.Sprintf("iokit:%x:-", entry)
			nodes = append(nodes, n)
			continue
		}
		for _, f := range fns {
			n := base
			n.Path = fmt.Sprintf("iokit:%x:%d", entry, f.Interface)
			if len(fns) > 1 {
				n.Name = fmt.Sprintf("%s (interface %d)", base.Name, f.Interface)
				n.StableID = fmt.Sprintf("%s#%d", base.StableID, f.Interface)
			}
			nodes = append(nodes, n)
		}
	}
	portless(nodes)
	return nodes, nil
}

func videoFunctions(entry uint64) ([]vcFunction, error) {
	d, err := openDev(entry)
	if err != nil {
		return nil, err
	}
	defer C.pl_close(d)
	return readFunctions(d)
}

func openDev(entry uint64) (C.pl_dev, error) {
	var d C.pl_dev
	if err := ioErr("open USB device", C.pl_open(C.uint64_t(entry), &d)); err != nil {
		return nil, err
	}
	return d, nil
}

func readFunctions(d C.pl_dev) ([]vcFunction, error) {
	buf := C.malloc(65535)
	defer C.free(buf)
	n := C.int(65535)
	if err := ioErr("GetConfigurationDescriptorPtr", C.pl_config(d, buf, &n)); err != nil {
		return nil, err
	}
	return parseVideoControl(C.GoBytes(buf, n)), nil
}

func parsePath(path string) (entry uint64, iface int, err error) {
	parts := strings.Split(path, ":")
	if len(parts) != 3 || parts[0] != "iokit" {
		return 0, 0, fmt.Errorf("%w: bad path %q", ErrNotFound, path)
	}
	if entry, err = strconv.ParseUint(parts[1], 16, 64); err != nil {
		return 0, 0, fmt.Errorf("%w: bad path %q", ErrNotFound, path)
	}
	if parts[2] == "-" {
		return entry, -1, nil
	}
	if iface, err = strconv.Atoi(parts[2]); err != nil {
		return 0, 0, fmt.Errorf("%w: bad path %q", ErrNotFound, path)
	}
	return entry, iface, nil
}

func (iokit) Open(path string) (Control, error) {
	entry, iface, err := parsePath(path)
	if err != nil {
		return nil, err
	}
	d, err := openDev(entry)
	if err != nil {
		if e, ok := err.(*IOReturnError); ok && e.Code == 0xe00002c0 {
			return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		return nil, err
	}
	fns, err := readFunctions(d)
	if err != nil {
		C.pl_close(d)
		return nil, err
	}
	for _, f := range fns {
		if int(f.Interface) != iface {
			continue
		}
		return &control{dev: d, fn: f}, nil
	}
	C.pl_close(d)
	return nil, fmt.Errorf("%w: no Video Control interface %d", ErrNoZoom, iface)
}

type control struct {
	mu  sync.Mutex
	dev C.pl_dev
	fn  vcFunction
	buf unsafe.Pointer // C memory for request data
}

func (c *control) index() C.uint16_t {
	return C.uint16_t(uint16(c.fn.TerminalID)<<8 | uint16(c.fn.Interface))
}

// get issues a zoom GET_xxx request. Callers hold c.mu.
func (c *control) get(req uint8, op string) (int32, error) {
	if c.dev == nil {
		return 0, ErrClosed
	}
	if !c.fn.HasCamera || !c.fn.Zoom {
		return 0, ErrNoZoom
	}
	if c.buf == nil {
		c.buf = C.malloc(zoomAbsoluteLength)
	}
	var done C.uint32_t
	kr := C.pl_request(c.dev, reqTypeClassIfIn, C.uint8_t(req), ctZoomAbsolute<<8, c.index(), c.buf, zoomAbsoluteLength, &done)
	if uint32(kr) == ioReturnAborted {
		// Seen once on the Link 2 Pro reading back straight after a SET_CUR;
		// a second attempt succeeds.
		kr = C.pl_request(c.dev, reqTypeClassIfIn, C.uint8_t(req), ctZoomAbsolute<<8, c.index(), c.buf, zoomAbsoluteLength, &done)
	}
	if err := ioErr(op, kr); err != nil {
		return 0, err
	}
	if done != zoomAbsoluteLength {
		return 0, fmt.Errorf("uvc: %s: short read (%d bytes)", op, done)
	}
	b := C.GoBytes(c.buf, zoomAbsoluteLength)
	return int32(uint16(b[0]) | uint16(b[1])<<8), nil
}

func (c *control) ZoomRange() (Range, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	var r Range
	var err error
	if r.Min, err = c.get(uvcGetMin, "GET_MIN zoom"); err != nil {
		return Range{}, wrapNoZoom(err)
	}
	if r.Max, err = c.get(uvcGetMax, "GET_MAX zoom"); err != nil {
		return Range{}, wrapNoZoom(err)
	}
	if r.Step, err = c.get(uvcGetRes, "GET_RES zoom"); err != nil {
		r.Step = 1
	}
	if r.Default, err = c.get(uvcGetDef, "GET_DEF zoom"); err != nil {
		r.Default = r.Min
	}
	return r, nil
}

func wrapNoZoom(err error) error {
	if err == ErrNoZoom || err == ErrClosed {
		return err
	}
	return fmt.Errorf("%w: %v", ErrNoZoom, err)
}

func (c *control) Zoom() (int32, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.get(uvcGetCur, "GET_CUR zoom")
}

func (c *control) SetZoom(v int32) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dev == nil {
		return ErrClosed
	}
	if !c.fn.HasCamera || !c.fn.Zoom {
		return ErrNoZoom
	}
	if v < 0 {
		v = 0
	} else if v > 0xffff {
		v = 0xffff
	}
	if c.buf == nil {
		c.buf = C.malloc(zoomAbsoluteLength)
	}
	b := (*[zoomAbsoluteLength]byte)(c.buf)
	b[0], b[1] = byte(v), byte(v>>8)
	var done C.uint32_t
	kr := C.pl_request(c.dev, reqTypeClassIfOut, uvcSetCur, ctZoomAbsolute<<8, c.index(), c.buf, zoomAbsoluteLength, &done)
	return ioErr("SET_CUR zoom", kr)
}

func (c *control) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.dev == nil {
		return nil
	}
	C.pl_close(c.dev)
	c.dev = nil
	if c.buf != nil {
		C.free(c.buf)
		c.buf = nil
	}
	return nil
}
