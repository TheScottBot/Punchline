package main

import (
	"testing"

	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/camera/insta360"
	"github.com/thescottbot/punchline/internal/camera/powerconf"
	"github.com/thescottbot/punchline/internal/camera/streamcam"
	"github.com/thescottbot/punchline/internal/uvc"
	"github.com/thescottbot/punchline/internal/uvc/uvcfake"
)

// The supported models' zoom levels, as discovered and as raw commands. These
// pin the per-model numbers the menu and hotkeys depend on.
func TestModels(t *testing.T) {
	cases := []struct {
		name        string
		adapter     func(uvc.Backend) camera.Adapter
		vid, pid    uint16
		raw         uvc.Range
		max, half   float64
		wantID      string
		rawAtLevels map[float64]int32
	}{
		{
			name: "Insta360 Link 2 Pro", adapter: insta360.New, vid: 0x2e1a, pid: 0x4c06,
			raw: uvc.Range{Min: 100, Max: 400, Step: 1, Default: 100}, max: 4, half: 2,
			wantID:      "insta360:2e1a:4c06",
			rawAtLevels: map[float64]int32{1: 100, 2: 200, 4: 400},
		},
		{
			name: "Anker PowerConf C200", adapter: powerconf.New, vid: 0x291a, pid: 0x3369,
			raw: uvc.Range{Min: 100, Max: 400, Step: 1, Default: 100}, max: 4, half: 0,
			wantID:      "powerconf:291a:3369:SER",
			rawAtLevels: map[float64]int32{1: 100, 4: 400},
		},
		{
			name: "Logitech StreamCam", adapter: streamcam.New, vid: 0x046d, pid: 0x0893,
			raw: uvc.Range{Min: 100, Max: 200, Step: 1, Default: 100}, max: 2, half: 0,
			wantID:      "streamcam:046d:0893:SER",
			rawAtLevels: map[float64]int32{1: 100, 2: 200},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stable := c.wantID[len(c.adapter(&uvcfake.Backend{}).Name())+1:]
			fb := &uvcfake.Backend{
				Nodes: []uvc.Node{
					{Path: "other", Name: "Something else", VendorID: 0x1234, ProductID: 0x5678, StableID: "1234:5678:X"},
					{Path: "cam", Name: c.name, VendorID: c.vid, ProductID: c.pid, StableID: stable},
				},
				Range: c.raw,
			}
			a := c.adapter(fb)
			infos, err := a.Discover()
			if err != nil || len(infos) != 1 {
				t.Fatalf("discover: %v %+v", err, infos)
			}
			info := infos[0]
			if string(info.ID) != c.wantID || info.Model != c.name {
				t.Errorf("info %+v", info)
			}
			if info.Zoom.MaxMagnification != c.max || info.Zoom.HalfwayMagnification != c.half {
				t.Errorf("discovered zoom %+v; want max %g halfway %g", info.Zoom, c.max, c.half)
			}
			d, err := a.Open(info.ID)
			if err != nil {
				t.Fatal(err)
			}
			defer d.Close()
			if cp := d.Capability(); cp.MaxMagnification != c.max || cp.HalfwayMagnification != c.half {
				t.Errorf("capability %+v", cp)
			}
			for mag, want := range c.rawAtLevels {
				before := len(fb.Sets())
				if err := d.SetMagnification(mag); err != nil {
					t.Fatal(err)
				}
				if got := fb.Sets()[before]; got != want {
					t.Errorf("%gx -> raw %d; want %d", mag, got, want)
				}
				if m, _ := d.Magnification(); m != mag {
					t.Errorf("read back %gx; want %gx", m, mag)
				}
			}
		})
	}
}

func TestAllModelsWiredIn(t *testing.T) {
	got := map[string]bool{}
	for _, a := range adapters() {
		got[a.Name()] = true
	}
	for _, want := range []string{"powerconf", "streamcam", "insta360"} {
		if !got[want] {
			t.Errorf("adapter %q not wired into the app", want)
		}
	}
}
