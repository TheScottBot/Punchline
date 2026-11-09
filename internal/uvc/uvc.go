// Package uvc gives direct access to the standard USB Video Class zoom
// control (CT_ZOOM_ABSOLUTE) through each operating system's native camera
// control interface. It is one of only two packages allowed to touch the OS.
package uvc

import (
	"errors"
	"fmt"
	"strings"
)

// Node is a video capture device found by enumeration.
type Node struct {
	// Path opens the device with Backend.Open.
	Path string
	// Name is the OS friendly name.
	Name string
	// VendorID and ProductID are the USB IDs, zero if not a USB device.
	VendorID, ProductID uint16
	// StableID identifies the physical device across reconnects:
	// "vvvv:pppp:SERIAL" when the device reports a serial number, otherwise
	// "vvvv:pppp@<port-specific instance>".
	StableID string
}

// USBID formats the vendor and product IDs as "vvvv:pppp".
func (n Node) USBID() string { return fmt.Sprintf("%04x:%04x", n.VendorID, n.ProductID) }

// Range is the device's raw absolute zoom range.
type Range struct {
	Min, Max, Step, Default int32
}

// Control is an open handle on a device's zoom control.
type Control interface {
	ZoomRange() (Range, error)
	Zoom() (int32, error)
	SetZoom(v int32) error
	Close() error
}

// Backend enumerates and opens devices.
type Backend interface {
	Enumerate() ([]Node, error)
	Open(path string) (Control, error)
}

var (
	ErrUnsupportedPlatform = errors.New("uvc: not implemented on this platform")
	ErrNoZoom              = errors.New("uvc: device has no zoom control")
	ErrNotFound            = errors.New("uvc: device not found")
	ErrClosed              = errors.New("uvc: control closed")
)

// parseUSBIDs extracts vid_xxxx and pid_xxxx from a device path or instance
// ID, case-insensitively.
func parseUSBIDs(s string) (vid, pid uint16, ok bool) {
	l := strings.ToLower(s)
	vi := strings.Index(l, "vid_")
	pi := strings.Index(l, "pid_")
	if vi < 0 || pi < 0 || len(l) < vi+8 || len(l) < pi+8 {
		return 0, 0, false
	}
	var v, p uint16
	if _, err := fmt.Sscanf(l[vi+4:vi+8], "%04x", &v); err != nil {
		return 0, 0, false
	}
	if _, err := fmt.Sscanf(l[pi+4:pi+8], "%04x", &p); err != nil {
		return 0, 0, false
	}
	return v, p, true
}
