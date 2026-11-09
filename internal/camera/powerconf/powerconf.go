// Package powerconf supports the Anker PowerConf C200 through standard UVC.
package powerconf

import (
	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/camera/uvcadapter"
	"github.com/thescottbot/punchline/internal/uvc"
)

// Model is the Anker PowerConf C200 (USB 291a:3369). It reports a UVC zoom
// range of 100..400. Treating that as 1x..4x is provisional until checked
// on hardware with cmd/uvcprobe -sweep.
var Model = uvcadapter.Model{
	Name:             "Anker PowerConf C200",
	VendorID:         0x291a,
	ProductID:        0x3369,
	MaxMagnification: 4,
}

// New returns the PowerConf adapter.
func New(b uvc.Backend) camera.Adapter { return uvcadapter.New("powerconf", b, Model) }
