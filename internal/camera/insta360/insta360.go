// Package insta360 supports the Insta360 Link 2 Pro through standard UVC.
//
// The Link SDK's zoom calls (GetZoomAbsoluteRange / SetZoomAbsolute) are the
// standard UVC CT_ZOOM_ABSOLUTE control, which the camera exposes directly,
// so neither the SDK nor Insta360 Link Controller is needed.
package insta360

import (
	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/camera/uvcadapter"
	"github.com/thescottbot/punchline/internal/uvc"
)

// Link2Pro is the Insta360 Link 2 Pro (USB 2e1a:4c06). Probed on macOS: UVC
// zoom 100..400, step 1, default 100; every value in the range is accepted
// and read back, and a command completes in under 1 ms. A visual sweep
// confirmed a smooth zoom with 400 = 4x. The camera reports
// no USB serial number, so its ID is port-independent only while it is the
// only Link 2 Pro connected (see uvc.Node.StableID).
var Link2Pro = uvcadapter.Model{
	Name:             "Insta360 Link 2 Pro",
	VendorID:         0x2e1a,
	ProductID:        0x4c06,
	MaxMagnification: 4,
}

// New returns the Insta360 adapter.
func New(b uvc.Backend) camera.Adapter { return uvcadapter.New("insta360", b, Link2Pro) }
