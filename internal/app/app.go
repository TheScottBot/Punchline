// Package app is the application core: camera selection and reconnection,
// zoom dispatch, hotkey enable/disable and rebinding, and persistence. It
// depends only on interfaces; the desktop shell drives it and supplies the
// OS hotkey registrar.
//
// Threading: Press, SetEnabled, BeginRebind, CompleteRebind, CancelRebind,
// SelectCamera, Start and Shutdown are called from the desktop's UI thread
// (so Registrar calls happen there). Camera discovery and connection run on
// a monitor goroutine; zoom commands run on a per-device worker goroutine.
// Snapshot may be called from anywhere.
package app

import (
	"errors"
	"fmt"
	"io"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/settings"
	"github.com/thescottbot/punchline/internal/zoom"
)

// Registrar registers system-wide hotkeys. The core calls it only from
// Start, Shutdown, SetEnabled, CompleteRebind and SyncHotkeys, which the
// desktop shell calls on its UI thread.
type Registrar interface {
	// Register binds key to action, replacing any existing registration for
	// action. On error the previous registration must remain in effect.
	Register(a hotkey.Action, k hotkey.Key) error
	// Unregister removes action's registration, if any.
	Unregister(a hotkey.Action)
}

// Store persists settings.
type Store interface {
	Save(settings.Settings) error
}

// Status is the camera connection state.
type Status int

const (
	StatusSearching    Status = iota // before the first scan
	StatusNoCamera                   // no supported camera connected
	StatusSelectCamera               // several cameras, none chosen
	StatusWaiting                    // chosen camera not connected
	StatusConnecting
	StatusConnected
	StatusError
)

func (s Status) String() string {
	return [...]string{"Searching", "No supported camera", "Select a camera", "Disconnected", "Connecting", "Connected", "Error"}[s]
}

// Options configures a Core.
type Options struct {
	Adapters  []camera.Adapter
	Registrar Registrar
	Store     Store
	Settings  settings.Settings
	Logger    *log.Logger
	// OnChange is called (from any goroutine) after visible state changes.
	// The shell must respond by calling SyncHotkeys on its UI thread, then
	// refreshing its display.
	OnChange func()
	// PollInterval is the fallback rescan period; 0 disables polling.
	PollInterval time.Duration
}

// CameraEntry is a camera shown in the menu.
type CameraEntry struct {
	Info     camera.DeviceInfo
	Present  bool
	Selected bool
}

// ZoomLevel is one zoom action the selected camera offers.
type ZoomLevel struct {
	Action hotkey.Action
	// Magnification is the level the action selects, or 0 when the camera
	// has never been seen (its range is unknown).
	Magnification float64
}

// Snapshot is a consistent view of state for the UI.
type Snapshot struct {
	Cameras       []CameraEntry
	Selected      camera.DeviceInfo
	HasSelection  bool
	Status        Status
	Detail        string
	Bindings      hotkey.Bindings
	Enabled       bool
	Rebinding     bool
	RebindAction  hotkey.Action
	HotkeyProblem string
	// Zoom lists the selected camera's zoom actions in menu order: Normal
	// and Close-Up always, Halfway when the camera's model has one.
	Zoom []ZoomLevel
}

// Core is the application core.
type Core struct {
	opts Options
	log  *log.Logger

	mu            sync.Mutex
	set           settings.Settings
	known         map[camera.DeviceID]camera.DeviceInfo
	present       []camera.DeviceInfo
	conn          *connection
	status        Status
	detail        string
	lastScanErr   string
	rebinding     bool
	rebindAction  hotkey.Action
	hotkeyProblem string

	// regMu guards registered and lastSync. Held across Registrar calls,
	// which all happen on the shell's UI thread.
	regMu      sync.Mutex
	registered map[hotkey.Action]hotkey.Key
	lastSync   string

	scanMu   sync.Mutex
	rescanCh chan struct{}
	stop     chan struct{}
	done     chan struct{}
	started  bool
	stopOnce sync.Once
}

// New returns a Core. Call Start to begin.
func New(o Options) *Core {
	lg := o.Logger
	if lg == nil {
		lg = log.New(io.Discard, "", 0)
	}
	s := o.Settings.Clone()
	if s.Bindings().Validate() != nil {
		s = s.WithBindings(hotkey.DefaultBindings())
	}
	return &Core{
		opts:       o,
		log:        lg,
		set:        s,
		registered: map[hotkey.Action]hotkey.Key{},
		known:      map[camera.DeviceID]camera.DeviceInfo{},
		rescanCh:   make(chan struct{}, 1),
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// Start registers hotkeys (if enabled) and starts camera monitoring.
// It returns hotkey registration problems; the app keeps running regardless.
func (c *Core) Start() error {
	err := c.syncHotkeys(true)
	c.mu.Lock()
	c.started = true
	c.mu.Unlock()
	go c.monitor()
	return err
}

// Shutdown stops monitoring, unregisters hotkeys and releases the camera.
func (c *Core) Shutdown() {
	c.stopOnce.Do(func() {
		c.mu.Lock()
		started := c.started
		c.mu.Unlock()
		close(c.stop)
		if started {
			<-c.done
		}
		c.regMu.Lock()
		for a := range c.registered {
			c.opts.Registrar.Unregister(a)
			delete(c.registered, a)
		}
		c.lastSync = "stopped"
		c.regMu.Unlock()
		c.scanMu.Lock()
		c.mu.Lock()
		conn := c.conn
		c.conn = nil
		c.mu.Unlock()
		if conn != nil {
			conn.close()
		}
		c.scanMu.Unlock()
		c.log.Printf("shutdown complete")
	})
}

// Settings returns a copy of the current settings.
func (c *Core) Settings() settings.Settings {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.set.Clone()
}

// Snapshot returns the state for display.
func (c *Core) Snapshot() Snapshot {
	c.mu.Lock()
	defer c.mu.Unlock()
	sn := Snapshot{
		Status:        c.status,
		Detail:        c.detail,
		Bindings:      c.set.Bindings(),
		Enabled:       c.set.HotkeysEnabled,
		Rebinding:     c.rebinding,
		RebindAction:  c.rebindAction,
		HotkeyProblem: c.hotkeyProblem,
	}
	pref := camera.DeviceID(c.set.PreferredCamera)
	seen := false
	for _, d := range c.present {
		e := CameraEntry{Info: d, Present: true, Selected: d.ID == pref}
		seen = seen || e.Selected
		sn.Cameras = append(sn.Cameras, e)
	}
	if pref != "" {
		info, ok := c.known[pref]
		if !ok {
			info = camera.DeviceInfo{ID: pref, Name: "Saved camera"}
		}
		sn.Selected, sn.HasSelection = info, true
		if !seen {
			sn.Cameras = append(sn.Cameras, CameraEntry{Info: info, Selected: true})
		}
	}
	zc := c.selectedZoomLocked()
	for _, a := range c.offeredLocked(zc) {
		l := ZoomLevel{Action: a}
		if zc.MaxMagnification > 0 {
			l.Magnification = c.targetFor(a, pref, zc)
		}
		sn.Zoom = append(sn.Zoom, l)
	}
	return sn
}

// selectedZoomLocked is the selected camera's zoom capability: from the open
// device, else from discovery, else zero (never seen).
func (c *Core) selectedZoomLocked() camera.ZoomCapability {
	pref := camera.DeviceID(c.set.PreferredCamera)
	if c.conn != nil && c.conn.info.ID == pref {
		return c.conn.cap
	}
	return c.known[pref].Zoom
}

// offeredLocked lists the actions a camera with capability zc offers.
func (c *Core) offeredLocked(zc camera.ZoomCapability) []hotkey.Action {
	out := make([]hotkey.Action, 0, len(hotkey.Actions))
	for _, a := range hotkey.Actions {
		if a != hotkey.Halfway || zc.HalfwayMagnification > 0 {
			out = append(out, a)
		}
	}
	return out
}

// SyncHotkeys registers exactly the hotkeys the selected camera offers
// (none while disabled or rebinding). Call it on the UI thread whenever
// OnChange fires; it does nothing when nothing relevant changed, so a key
// that failed to register is not retried until the bindings, the camera's
// levels or the enabled state change.
func (c *Core) SyncHotkeys() { c.syncHotkeys(false) }

func (c *Core) syncHotkeys(force bool) error {
	c.mu.Lock()
	if c.rebinding {
		c.mu.Unlock()
		return nil
	}
	b := c.set.Bindings()
	want := map[hotkey.Action]bool{}
	if c.set.HotkeysEnabled {
		for _, a := range c.offeredLocked(c.selectedZoomLocked()) {
			// MIDI-bound actions need no OS hot key.
			want[a] = !b.Get(a).IsMIDI()
		}
	}
	c.mu.Unlock()

	sig := fmt.Sprint(b, want)
	c.regMu.Lock()
	if c.lastSync == "stopped" || (!force && sig == c.lastSync) {
		c.regMu.Unlock()
		return nil
	}
	c.lastSync = sig
	var errs []error
	for _, a := range hotkey.Actions {
		k, had := c.registered[a]
		switch {
		case want[a] && (!had || k != b.Get(a)):
			if err := c.opts.Registrar.Register(a, b.Get(a)); err != nil {
				errs = append(errs, fmt.Errorf("%s (%s): %w", a, b.Get(a), err))
			} else {
				c.registered[a] = b.Get(a)
			}
		case !want[a] && had:
			c.opts.Registrar.Unregister(a)
			delete(c.registered, a)
		}
	}
	c.regMu.Unlock()

	err := errors.Join(errs...)
	problem := ""
	if err != nil {
		problem = err.Error()
		c.log.Printf("hotkey registration: %v", err)
	}
	c.mu.Lock()
	changed := c.hotkeyProblem != problem
	c.hotkeyProblem = problem
	c.mu.Unlock()
	if changed && !force {
		c.notify()
	}
	return err
}

// TriggerRescan asks the monitor to rescan soon (e.g. on a device change).
func (c *Core) TriggerRescan() {
	select {
	case c.rescanCh <- struct{}{}:
	default:
	}
}

func (c *Core) monitor() {
	defer close(c.done)
	var tick <-chan time.Time
	if c.opts.PollInterval > 0 {
		t := time.NewTicker(c.opts.PollInterval)
		defer t.Stop()
		tick = t.C
	}
	c.Rescan()
	for {
		select {
		case <-c.stop:
			return
		case <-c.rescanCh:
			c.Rescan()
		case <-tick:
			c.Rescan()
		}
	}
}

// Press applies an action's zoom. It never blocks on the camera: the command
// is queued and supersedes any not-yet-sent command.
func (c *Core) Press(a hotkey.Action) {
	at := time.Now()
	c.mu.Lock()
	if !c.set.HotkeysEnabled || c.rebinding {
		c.mu.Unlock()
		return
	}
	conn := c.conn
	if conn == nil {
		st := c.status
		c.mu.Unlock()
		c.log.Printf("%s pressed: no camera (%s)", a, st)
		return
	}
	if a == hotkey.Halfway && conn.cap.HalfwayMagnification == 0 {
		c.mu.Unlock()
		c.log.Printf("%s pressed: %s has no halfway level", a, conn.info.Name)
		return
	}
	mag := c.targetLocked(a, conn)
	c.mu.Unlock()
	conn.submit(request{action: a, mag: mag, at: at})
}

// PressBinding presses the action bound to k, for bindings that arrive
// without an OS hot key (MIDI triggers). Safe from any goroutine and never
// blocks on the camera. It reports whether an action is bound to k.
func (c *Core) PressBinding(k hotkey.Key) bool {
	c.mu.Lock()
	b := c.set.Bindings()
	c.mu.Unlock()
	for _, a := range hotkey.Actions {
		if b.Get(a) == k {
			c.Press(a)
			return true
		}
	}
	return false
}

// Rebinding reports whether a rebind is in progress. Safe from any goroutine.
func (c *Core) Rebinding() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rebinding
}

// Target returns the magnification an action selects on the current camera.
func (c *Core) Target(a hotkey.Action) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return 0, false
	}
	return c.targetLocked(a, c.conn), true
}

func (c *Core) targetLocked(a hotkey.Action, conn *connection) float64 {
	return c.targetFor(a, conn.info.ID, conn.cap)
}

// targetFor resolves an action's magnification for camera id: per-camera
// override, else the global setting, else the camera's own level.
func (c *Core) targetFor(a hotkey.Action, id camera.DeviceID, zc camera.ZoomCapability) float64 {
	ov := c.set.CameraZoom[string(id)]
	maxMag := zc.MaxMagnification
	var m float64
	switch a {
	case hotkey.Halfway:
		m = c.set.HalfwayZoom
		if ov.Halfway > 0 {
			m = ov.Halfway
		}
		if m == 0 {
			m = zc.HalfwayMagnification
		}
		if m == 0 {
			m = 1
		}
	case hotkey.CloseUp:
		m = c.set.CloseUpZoom
		if ov.CloseUp > 0 {
			m = ov.CloseUp
		}
		if m == 0 {
			m = maxMag
		}
	default:
		m = c.set.NormalZoom
		if ov.Normal > 0 {
			m = ov.Normal
		}
	}
	return zoom.Clamp(m, maxMag)
}

// SelectCamera makes id the preferred camera and connects to it.
func (c *Core) SelectCamera(id camera.DeviceID) {
	c.mu.Lock()
	if c.set.PreferredCamera == string(id) {
		c.mu.Unlock()
		return
	}
	c.set.PreferredCamera = string(id)
	s := c.set.Clone()
	c.mu.Unlock()
	c.log.Printf("camera selected: %s", id)
	c.save(s)
	c.notify()
	c.TriggerRescan()
}

// SetEnabled enables or disables both hotkeys and persists the choice.
func (c *Core) SetEnabled(on bool) error {
	c.mu.Lock()
	c.set.HotkeysEnabled = on
	s := c.set.Clone()
	c.mu.Unlock()
	err := c.syncHotkeys(true)
	c.log.Printf("hotkeys enabled=%v", on)
	c.save(s)
	c.notify()
	return err
}

// BeginRebind enters rebinding for a. Presses are ignored until it ends.
func (c *Core) BeginRebind(a hotkey.Action) {
	c.mu.Lock()
	c.rebinding, c.rebindAction = true, a
	c.mu.Unlock()
	c.notify()
}

// CancelRebind leaves rebinding without changes.
func (c *Core) CancelRebind() {
	c.mu.Lock()
	c.rebinding = false
	c.mu.Unlock()
	c.notify()
}

// CompleteRebind binds the action being rebound to k. The binding is
// validated, registered (or, while hotkeys are disabled, test-registered)
// and persisted only after registration succeeds. On any error the previous
// binding stays in effect. Rebinding ends either way.
func (c *Core) CompleteRebind(k hotkey.Key) error {
	c.mu.Lock()
	if !c.rebinding {
		c.mu.Unlock()
		return errors.New("not rebinding")
	}
	a := c.rebindAction
	b := c.set.Bindings()
	enabled := c.set.HotkeysEnabled
	c.rebinding = false
	c.mu.Unlock()
	defer c.notify()

	if err := hotkey.CheckRebind(b, a, k); err != nil {
		c.log.Printf("rebind %s rejected: %v", a, err)
		return err
	}
	if b.Get(a) == k {
		return nil
	}
	c.regMu.Lock()
	if k.IsMIDI() {
		// A MIDI trigger needs no registration; release the action's key.
		if _, had := c.registered[a]; had {
			c.opts.Registrar.Unregister(a)
			delete(c.registered, a)
		}
		c.regMu.Unlock()
		c.mu.Lock()
		c.set = c.set.WithBindings(b.With(a, k))
		s := c.set.Clone()
		c.mu.Unlock()
		c.log.Printf("rebound %s: %s -> %s", a, b.Get(a), k)
		c.save(s)
		return nil
	}
	if err := c.opts.Registrar.Register(a, k); err != nil {
		c.regMu.Unlock()
		c.log.Printf("rebind %s to %s failed: %v", a, k, err)
		return fmt.Errorf("%s: %w", k, err)
	}
	c.registered[a] = k
	c.mu.Lock()
	offered := false
	for _, o := range c.offeredLocked(c.selectedZoomLocked()) {
		offered = offered || o == a
	}
	c.mu.Unlock()
	if !enabled || !offered {
		// Registration was only a test that the key is free.
		c.opts.Registrar.Unregister(a)
		delete(c.registered, a)
	}
	c.regMu.Unlock()
	c.mu.Lock()
	c.set = c.set.WithBindings(b.With(a, k))
	s := c.set.Clone()
	c.mu.Unlock()
	c.log.Printf("rebound %s: %s -> %s", a, b.Get(a), k)
	c.save(s)
	return nil
}

func (c *Core) save(s settings.Settings) {
	if c.opts.Store == nil {
		return
	}
	if err := c.opts.Store.Save(s); err != nil {
		c.log.Printf("saving settings: %v", err)
	}
}

func (c *Core) notify() {
	if c.opts.OnChange != nil {
		c.opts.OnChange()
	}
}

func findInfo(list []camera.DeviceInfo, id camera.DeviceID) (camera.DeviceInfo, bool) {
	for _, d := range list {
		if d.ID == id {
			return d, true
		}
	}
	return camera.DeviceInfo{}, false
}

// Rescan discovers cameras and reconciles the connection. It never switches
// away from a chosen camera that is merely disconnected.
func (c *Core) Rescan() {
	c.scanMu.Lock()
	defer c.scanMu.Unlock()

	var found []camera.DeviceInfo
	var scanErrs []error
	adapters := map[string]camera.Adapter{}
	for _, a := range c.opts.Adapters {
		adapters[a.Name()] = a
		infos, err := a.Discover()
		if err != nil {
			scanErrs = append(scanErrs, fmt.Errorf("%s: %w", a.Name(), err))
			continue
		}
		found = append(found, infos...)
	}
	scanErr := errors.Join(scanErrs...)

	c.mu.Lock()
	if msg := fmt.Sprint(scanErr); scanErr != nil && msg != c.lastScanErr {
		c.log.Printf("camera discovery: %v", scanErr)
		c.lastScanErr = msg
	} else if scanErr == nil {
		c.lastScanErr = ""
	}
	c.present = found
	for _, d := range found {
		c.known[d.ID] = d
	}
	pref := camera.DeviceID(c.set.PreferredCamera)
	var save *settings.Settings
	if pref == "" && len(found) == 1 {
		pref = found[0].ID
		c.set.PreferredCamera = string(pref)
		s := c.set.Clone()
		save = &s
		c.log.Printf("auto-selected only camera: %s (%s)", found[0].Name, pref)
	}
	target, haveTarget := findInfo(found, pref)

	var toClose *connection
	if c.conn != nil && (!haveTarget || c.conn.info.ID != target.ID || c.conn.failed.Load()) {
		toClose = c.conn
		c.conn = nil
	}
	needOpen := haveTarget && c.conn == nil
	prevStatus := c.status
	switch {
	case c.conn != nil:
	case needOpen:
		c.status, c.detail = StatusConnecting, ""
	case scanErr != nil && len(found) == 0:
		c.status, c.detail = StatusError, scanErr.Error()
	case pref != "":
		c.status, c.detail = StatusWaiting, ""
	case len(found) == 0:
		c.status, c.detail = StatusNoCamera, ""
	default:
		c.status, c.detail = StatusSelectCamera, ""
	}
	changed := prevStatus != c.status
	c.mu.Unlock()

	if save != nil {
		c.save(*save)
	}
	if toClose != nil {
		toClose.close()
		c.log.Printf("camera released: %s", toClose.info.Name)
	}
	if needOpen {
		c.connect(adapters[target.Adapter], target)
		changed = true
	}
	if changed || save != nil {
		c.notify()
	}
}

func (c *Core) connect(a camera.Adapter, info camera.DeviceInfo) {
	var dev camera.Device
	err := fmt.Errorf("adapter %q not available", info.Adapter)
	if a != nil {
		dev, err = a.Open(info.ID)
	}
	c.mu.Lock()
	if err != nil {
		msg := err.Error()
		if c.status != StatusError || c.detail != msg {
			c.log.Printf("connecting %s: %v", info.Name, err)
		}
		c.status, c.detail = StatusError, msg
		c.mu.Unlock()
		return
	}
	if c.set.PreferredCamera != string(info.ID) {
		c.mu.Unlock()
		dev.Close()
		return
	}
	conn := newConnection(c, dev)
	c.conn = conn
	c.status, c.detail = StatusConnected, ""
	mag := c.targetLocked(hotkey.Normal, conn)
	c.mu.Unlock()
	c.log.Printf("camera connected: %s; %s", info.Name, conn.cap.Description)
	// Start from a known state: normal framing.
	conn.submit(request{action: hotkey.Normal, mag: mag, at: time.Now(), quiet: true})
}

func (c *Core) deviceFailed(conn *connection, err error) {
	c.mu.Lock()
	if c.conn != conn {
		c.mu.Unlock()
		return
	}
	conn.failed.Store(true)
	c.status, c.detail = StatusError, err.Error()
	c.mu.Unlock()
	c.notify()
	c.TriggerRescan()
}

type request struct {
	action hotkey.Action
	mag    float64
	at     time.Time
	quiet  bool
}

// connection owns an open device and a worker that applies the most recent
// zoom request. Older pending requests are superseded, never queued.
type connection struct {
	core   *Core
	info   camera.DeviceInfo
	dev    camera.Device
	cap    camera.ZoomCapability
	mu     sync.Mutex
	req    chan request
	quit   chan struct{}
	done   chan struct{}
	failed atomic.Bool
	once   sync.Once
}

func newConnection(c *Core, dev camera.Device) *connection {
	conn := &connection{
		core: c,
		info: dev.Info(),
		dev:  dev,
		cap:  dev.Capability(),
		req:  make(chan request, 1),
		quit: make(chan struct{}),
		done: make(chan struct{}),
	}
	go conn.run()
	return conn
}

func (cn *connection) submit(r request) {
	cn.mu.Lock()
	defer cn.mu.Unlock()
	select {
	case old := <-cn.req:
		cn.core.log.Printf("%s superseded by %s", old.action, r.action)
	default:
	}
	cn.req <- r
}

func (cn *connection) run() {
	defer close(cn.done)
	for {
		select {
		case <-cn.quit:
			return
		case r := <-cn.req:
			start := time.Now()
			err := cn.dev.SetMagnification(r.mag)
			if err != nil {
				cn.core.log.Printf("%s %.2fx on %s failed: %v", r.action, r.mag, cn.info.Name, err)
				cn.core.deviceFailed(cn, err)
				continue
			}
			if !r.quiet {
				cn.core.log.Printf("%s %.2fx: queued %v, command %v", r.action, r.mag, start.Sub(r.at), time.Since(start))
			}
		}
	}
}

func (cn *connection) close() {
	cn.once.Do(func() {
		close(cn.quit)
		<-cn.done
		cn.dev.Close()
	})
}
