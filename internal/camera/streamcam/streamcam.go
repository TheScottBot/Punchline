// Package streamcam supports the Logitech StreamCam through standard UVC.
package streamcam

import (
	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/camera/uvcadapter"
	"github.com/thescottbot/punchline/internal/uvc"
)

// Model is the Logitech StreamCam (USB 046d:0893). Its UVC range is
// continuous but only two crop levels are visible: 1x and 2x (per
// nf/logicam; to be verified on hardware with cmd/uvcprobe -sweep).
var Model = uvcadapter.Model{
	Name:             "Logitech StreamCam",
	VendorID:         0x046d,
	ProductID:        0x0893,
	MaxMagnification: 2,
}

// New returns the StreamCam adapter.
func New(b uvc.Backend) camera.Adapter { return uvcadapter.New("streamcam", b, Model) }
