// Package camera defines the interface every webcam adapter implements.
// The application core depends only on these types: never on SDK types or
// raw device zoom values.
package camera

import "errors"

// DeviceID identifies a physical camera stably across reconnects where the
// hardware allows (for USB, vendor:product:serial).
type DeviceID string

// DeviceInfo describes a discovered, supported camera.
type DeviceInfo struct {
	ID      DeviceID
	Name    string // friendly name for menus
	Model   string // e.g. "Logitech StreamCam"
	Adapter string // name of the adapter that found it
}

// ZoomCapability describes a device's zoom in logical terms.
type ZoomCapability struct {
	// MaxMagnification is the largest supported magnification (1 = none).
	MaxMagnification float64
	// Description is free text for diagnostics, e.g. the raw range.
	Description string
}

// Device is an open camera.
//
// Implementations must be safe for use from one goroutine at a time; the
// core serialises calls per device.
type Device interface {
	Info() DeviceInfo
	Capability() ZoomCapability
	// SetMagnification issues an absolute zoom command. mag is clamped to
	// [1, MaxMagnification].
	SetMagnification(mag float64) error
	// Magnification reads the current zoom, or ErrUnsupported.
	Magnification() (float64, error)
	// Close releases the device. It is safe to call more than once.
	Close() error
}

// Adapter discovers and opens one family of cameras.
type Adapter interface {
	Name() string
	// Discover lists currently connected supported cameras.
	Discover() ([]DeviceInfo, error)
	// Open connects to a camera returned by Discover.
	Open(id DeviceID) (Device, error)
}

var (
	ErrNotFound        = errors.New("camera not found")
	ErrUnsupported     = errors.New("operation not supported by camera")
	ErrZoomUnsupported = errors.New("camera does not expose a zoom control")
	ErrClosed          = errors.New("camera closed")
)
