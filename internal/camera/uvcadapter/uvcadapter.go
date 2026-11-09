// Package uvcadapter implements camera.Adapter for cameras whose zoom is
// reachable through the standard UVC zoom control, for a fixed set of models.
package uvcadapter

import (
	"fmt"
	"sync"

	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/uvc"
	"github.com/thescottbot/punchline/internal/zoom"
)

// Model is a supported camera model.
type Model struct {
	Name                string
	VendorID, ProductID uint16
	// MaxMagnification is the magnification at the top of the device's raw
	// zoom range. UVC does not report it, so it is known per model.
	MaxMagnification float64
	// HalfwayMagnification is an optional intermediate level (0 = none),
	// for cameras whose full close-up is too much for some uses.
	HalfwayMagnification float64
}

func (m Model) capability() camera.ZoomCapability {
	return camera.ZoomCapability{MaxMagnification: m.MaxMagnification, HalfwayMagnification: m.HalfwayMagnification}
}

// Adapter discovers and opens the listed models through a uvc.Backend.
type Adapter struct {
	name    string
	backend uvc.Backend
	models  []Model
}

// New returns an adapter named name for the given models.
func New(name string, backend uvc.Backend, models ...Model) *Adapter {
	return &Adapter{name: name, backend: backend, models: models}
}

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) match(n uvc.Node) (Model, bool) {
	for _, m := range a.models {
		if n.VendorID == m.VendorID && n.ProductID == m.ProductID {
			return m, true
		}
	}
	return Model{}, false
}

func (a *Adapter) id(n uvc.Node) camera.DeviceID {
	return camera.DeviceID(a.name + ":" + n.StableID)
}

type found struct {
	node  uvc.Node
	model Model
	info  camera.DeviceInfo
}

func (a *Adapter) scan() ([]found, error) {
	nodes, err := a.backend.Enumerate()
	if err != nil {
		return nil, err
	}
	var out []found
	count := map[string]int{}
	for _, n := range nodes {
		m, ok := a.match(n)
		if !ok {
			continue
		}
		count[m.Name]++
		name := m.Name
		if count[m.Name] > 1 {
			name = fmt.Sprintf("%s (%d)", m.Name, count[m.Name])
		}
		out = append(out, found{n, m, camera.DeviceInfo{ID: a.id(n), Name: name, Model: m.Name, Adapter: a.name, Zoom: m.capability()}})
	}
	return out, nil
}

func (a *Adapter) Discover() ([]camera.DeviceInfo, error) {
	fs, err := a.scan()
	if err != nil {
		return nil, err
	}
	out := make([]camera.DeviceInfo, len(fs))
	for i, f := range fs {
		out[i] = f.info
	}
	return out, nil
}

func (a *Adapter) Open(id camera.DeviceID) (camera.Device, error) {
	fs, err := a.scan()
	if err != nil {
		return nil, err
	}
	for _, f := range fs {
		if f.info.ID != id {
			continue
		}
		m, info := f.model, f.info
		c, err := a.backend.Open(f.node.Path)
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", m.Name, err)
		}
		ur, err := c.ZoomRange()
		r := zoom.Range(ur)
		if err != nil || !r.Valid() {
			c.Close()
			return nil, fmt.Errorf("%s: %w (range %+v, %v)", m.Name, camera.ErrZoomUnsupported, ur, err)
		}
		return &device{info: info, model: m, ctl: c, r: r}, nil
	}
	return nil, fmt.Errorf("%s: %w", id, camera.ErrNotFound)
}

type device struct {
	mu    sync.Mutex
	info  camera.DeviceInfo
	model Model
	ctl   uvc.Control
	r     zoom.Range
}

func (d *device) Info() camera.DeviceInfo { return d.info }

func (d *device) Capability() camera.ZoomCapability {
	c := d.model.capability()
	c.Description = fmt.Sprintf("UVC zoom raw %d..%d step %d default %d = 1x..%gx",
		d.r.Min, d.r.Max, d.r.Step, d.r.Default, d.model.MaxMagnification)
	if c.HalfwayMagnification > 0 {
		c.Description += fmt.Sprintf(", halfway %gx", c.HalfwayMagnification)
	}
	return c
}

func (d *device) SetMagnification(mag float64) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ctl.SetZoom(zoom.ToRaw(mag, d.model.MaxMagnification, d.r))
}

func (d *device) Magnification() (float64, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	v, err := d.ctl.Zoom()
	if err != nil {
		return 0, err
	}
	return zoom.FromRaw(v, d.model.MaxMagnification, d.r), nil
}

func (d *device) Close() error { return d.ctl.Close() }
