package hwtest

import (
	"math"
	"os"
	"testing"
	"time"

	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/camera/insta360"
	"github.com/thescottbot/punchline/internal/camera/powerconf"
	"github.com/thescottbot/punchline/internal/camera/streamcam"
	"github.com/thescottbot/punchline/internal/uvc"
)

// maxCommand bounds one absolute zoom command. Measured: < 1 ms on the
// Link 2 Pro (macOS), 1-26 ms on the C200 (Windows).
const maxCommand = 100 * time.Millisecond

func TestRealCameras(t *testing.T) {
	if os.Getenv("PUNCHLINE_HW") != "1" {
		t.Skip("set PUNCHLINE_HW=1 with a supported camera connected")
	}
	b := uvc.System()
	adapters := []camera.Adapter{powerconf.New(b), streamcam.New(b), insta360.New(b)}
	found := 0
	for _, a := range adapters {
		infos, err := a.Discover()
		if err != nil {
			t.Fatalf("%s discover: %v", a.Name(), err)
		}
		for _, info := range infos {
			found++
			t.Run(info.Name, func(t *testing.T) { exercise(t, a, info) })
		}
	}
	if found == 0 {
		t.Fatal("no supported camera connected")
	}
}

func exercise(t *testing.T, a camera.Adapter, info camera.DeviceInfo) {
	t0 := time.Now()
	d, err := a.Open(info.ID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Logf("%s: opened in %v; %s", info.ID, time.Since(t0), d.Capability().Description)
	defer d.Close()
	defer d.SetMagnification(1) // leave normal framing

	cp := d.Capability()
	if cp != (camera.ZoomCapability{MaxMagnification: info.Zoom.MaxMagnification, HalfwayMagnification: info.Zoom.HalfwayMagnification, Description: cp.Description}) {
		t.Errorf("open capability %+v differs from discovery %+v", cp, info.Zoom)
	}
	levels := []float64{1}
	if cp.HalfwayMagnification > 0 {
		levels = append(levels, cp.HalfwayMagnification)
	}
	levels = append(levels, cp.MaxMagnification, 1)
	for _, mag := range levels {
		t0 := time.Now()
		if err := d.SetMagnification(mag); err != nil {
			t.Fatalf("set %gx: %v", mag, err)
		}
		took := time.Since(t0)
		got, err := d.Magnification()
		if err != nil {
			t.Fatalf("read back after %gx: %v", mag, err)
		}
		// Allow one raw step of rounding over the whole range.
		if math.Abs(got-mag) > 0.02*cp.MaxMagnification {
			t.Errorf("set %gx, read back %.3fx", mag, got)
		}
		if took > maxCommand {
			t.Errorf("set %gx took %v (> %v)", mag, took, maxCommand)
		}
		t.Logf("  %gx: command %v, read back %.3fx", mag, took, got)
		time.Sleep(300 * time.Millisecond) // visible on a preview
	}
}
