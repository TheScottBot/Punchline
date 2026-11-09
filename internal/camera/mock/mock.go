// Package mock provides an in-memory camera adapter for tests.
package mock

import (
	"fmt"
	"sync"

	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/zoom"
)

// Adapter is a controllable fake camera adapter.
type Adapter struct {
	mu      sync.Mutex
	name    string
	devices map[camera.DeviceID]*state
	order   []camera.DeviceID
	discErr error
	openErr error
	setErr  error
	setHook func(mag float64) // called inside SetMagnification, before recording
	opens   int
}

type state struct {
	info    camera.DeviceInfo
	maxMag  float64
	present bool
	calls   []float64
	current float64
	open    int
}

// New returns an empty adapter.
func New(name string) *Adapter {
	return &Adapter{name: name, devices: map[camera.DeviceID]*state{}}
}

// Add registers a connected camera.
func (a *Adapter) Add(id, name string, maxMag float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	did := camera.DeviceID(id)
	if _, ok := a.devices[did]; !ok {
		a.order = append(a.order, did)
	}
	a.devices[did] = &state{
		info:    camera.DeviceInfo{ID: did, Name: name, Model: name, Adapter: a.name, Zoom: camera.ZoomCapability{MaxMagnification: maxMag}},
		maxMag:  maxMag,
		present: true,
		current: 1,
	}
}

// SetHalfway gives a camera a halfway level (0 removes it).
func (a *Adapter) SetHalfway(id string, mag float64) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.devices[camera.DeviceID(id)].info.Zoom.HalfwayMagnification = mag
}

// SetPresent simulates unplugging or replugging a camera.
func (a *Adapter) SetPresent(id string, present bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.devices[camera.DeviceID(id)].present = present
}

// FailDiscover makes Discover return err (nil clears it).
func (a *Adapter) FailDiscover(err error) { a.mu.Lock(); a.discErr = err; a.mu.Unlock() }

// FailOpen makes Open return err (nil clears it).
func (a *Adapter) FailOpen(err error) { a.mu.Lock(); a.openErr = err; a.mu.Unlock() }

// FailSet makes SetMagnification return err (nil clears it).
func (a *Adapter) FailSet(err error) { a.mu.Lock(); a.setErr = err; a.mu.Unlock() }

// OnSet installs a hook run at the start of every SetMagnification call.
func (a *Adapter) OnSet(f func(mag float64)) { a.mu.Lock(); a.setHook = f; a.mu.Unlock() }

// Calls returns the magnifications sent to a camera, in order.
func (a *Adapter) Calls(id string) []float64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]float64(nil), a.devices[camera.DeviceID(id)].calls...)
}

// OpenCount returns how many handles to a camera are open.
func (a *Adapter) OpenCount(id string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.devices[camera.DeviceID(id)].open
}

// Opens returns the total number of successful Open calls.
func (a *Adapter) Opens() int { a.mu.Lock(); defer a.mu.Unlock(); return a.opens }

func (a *Adapter) Name() string { return a.name }

func (a *Adapter) Discover() ([]camera.DeviceInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.discErr != nil {
		return nil, a.discErr
	}
	var out []camera.DeviceInfo
	for _, id := range a.order {
		if s := a.devices[id]; s.present {
			out = append(out, s.info)
		}
	}
	return out, nil
}

func (a *Adapter) Open(id camera.DeviceID) (camera.Device, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.openErr != nil {
		return nil, a.openErr
	}
	s, ok := a.devices[id]
	if !ok || !s.present {
		return nil, fmt.Errorf("%s: %w", id, camera.ErrNotFound)
	}
	s.open++
	a.opens++
	return &device{a: a, s: s}, nil
}

type device struct {
	a      *Adapter
	s      *state
	closed bool
}

func (d *device) Info() camera.DeviceInfo { return d.s.info }

func (d *device) Capability() camera.ZoomCapability {
	c := d.s.info.Zoom
	c.Description = "mock"
	return c
}

func (d *device) SetMagnification(mag float64) error {
	d.a.mu.Lock()
	hook := d.a.setHook
	d.a.mu.Unlock()
	if hook != nil {
		hook(mag)
	}
	d.a.mu.Lock()
	defer d.a.mu.Unlock()
	if d.closed {
		return camera.ErrClosed
	}
	if !d.s.present {
		return fmt.Errorf("%s: %w", d.s.info.ID, camera.ErrNotFound)
	}
	if d.a.setErr != nil {
		return d.a.setErr
	}
	mag = zoom.Clamp(mag, d.s.maxMag)
	d.s.calls = append(d.s.calls, mag)
	d.s.current = mag
	return nil
}

func (d *device) Magnification() (float64, error) {
	d.a.mu.Lock()
	defer d.a.mu.Unlock()
	return d.s.current, nil
}

func (d *device) Close() error {
	d.a.mu.Lock()
	defer d.a.mu.Unlock()
	if !d.closed {
		d.closed = true
		d.s.open--
	}
	return nil
}
