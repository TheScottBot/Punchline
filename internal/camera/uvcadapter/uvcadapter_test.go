package uvcadapter

import (
	"errors"
	"testing"

	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/uvc"
)

type fakeBackend struct {
	nodes []uvc.Node
	r     uvc.Range
	sets  []int32
}

func (f *fakeBackend) Enumerate() ([]uvc.Node, error) { return f.nodes, nil }
func (f *fakeBackend) Open(p string) (uvc.Control, error) {
	for _, n := range f.nodes {
		if n.Path == p {
			return &fakeCtl{f: f}, nil
		}
	}
	return nil, uvc.ErrNotFound
}

type fakeCtl struct {
	f   *fakeBackend
	cur int32
}

func (c *fakeCtl) ZoomRange() (uvc.Range, error) { return c.f.r, nil }
func (c *fakeCtl) Zoom() (int32, error)          { return c.cur, nil }
func (c *fakeCtl) SetZoom(v int32) error         { c.cur = v; c.f.sets = append(c.f.sets, v); return nil }
func (c *fakeCtl) Close() error                  { return nil }

func TestDiscoverFiltersModelsAndMaps(t *testing.T) {
	fb := &fakeBackend{
		nodes: []uvc.Node{
			{Path: "a", Name: "Other", VendorID: 1, ProductID: 2, StableID: "0001:0002:x"},
			{Path: "b", Name: "Logitech StreamCam", VendorID: 0x046d, ProductID: 0x0893, StableID: "046d:0893:SER"},
		},
		r: uvc.Range{Min: 100, Max: 200, Step: 1},
	}
	a := New("streamcam", fb, Model{Name: "Logitech StreamCam", VendorID: 0x046d, ProductID: 0x0893, MaxMagnification: 2})
	infos, err := a.Discover()
	if err != nil || len(infos) != 1 || infos[0].ID != "streamcam:046d:0893:SER" {
		t.Fatalf("%v %+v", err, infos)
	}
	d, err := a.Open(infos[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	d.SetMagnification(2)
	d.SetMagnification(1)
	d.SetMagnification(9)
	if got := fb.sets; len(got) != 3 || got[0] != 200 || got[1] != 100 || got[2] != 200 {
		t.Fatalf("sets %v", got)
	}
	if _, err := a.Open("streamcam:nope"); !errors.Is(err, camera.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestNoZoomRange(t *testing.T) {
	fb := &fakeBackend{nodes: []uvc.Node{{Path: "b", VendorID: 1, ProductID: 1, StableID: "s"}}}
	a := New("x", fb, Model{Name: "M", VendorID: 1, ProductID: 1, MaxMagnification: 2})
	if _, err := a.Open("x:s"); !errors.Is(err, camera.ErrZoomUnsupported) {
		t.Fatalf("got %v", err)
	}
}
