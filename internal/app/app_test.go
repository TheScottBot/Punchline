package app

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/camera/mock"
	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/settings"
)

type fakeReg struct {
	mu    sync.Mutex
	keys  map[hotkey.Action]hotkey.Key
	taken map[hotkey.Key]bool
	calls int
}

func newReg() *fakeReg {
	return &fakeReg{keys: map[hotkey.Action]hotkey.Key{}, taken: map[hotkey.Key]bool{}}
}

func (r *fakeReg) Register(a hotkey.Action, k hotkey.Key) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if r.taken[k] {
		return hotkey.ErrUnavailable
	}
	r.keys[a] = k
	return nil
}

func (r *fakeReg) Unregister(a hotkey.Action) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.keys, a)
}

func (r *fakeReg) get(a hotkey.Action) (hotkey.Key, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	k, ok := r.keys[a]
	return k, ok
}

type fakeStore struct {
	mu    sync.Mutex
	saved []settings.Settings
}

func (s *fakeStore) Save(v settings.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saved = append(s.saved, v.Clone())
	return nil
}

func (s *fakeStore) last() (settings.Settings, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.saved) == 0 {
		return settings.Settings{}, false
	}
	return s.saved[len(s.saved)-1], true
}

type rig struct {
	t     *testing.T
	cam   *mock.Adapter
	reg   *fakeReg
	store *fakeStore
	core  *Core
}

func newRig(t *testing.T, s settings.Settings) *rig {
	r := &rig{t: t, cam: mock.New("mock"), reg: newReg(), store: &fakeStore{}}
	r.core = New(Options{Adapters: []camera.Adapter{r.cam}, Registrar: r.reg, Store: r.store, Settings: s})
	t.Cleanup(r.core.Shutdown)
	return r
}

// waitCalls waits until the camera has received want commands.
func (r *rig) waitCalls(id string, want []float64) {
	r.t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := r.cam.Calls(id)
		if len(got) == len(want) {
			for i := range got {
				if got[i] != want[i] {
					r.t.Fatalf("calls %v; want %v", got, want)
				}
			}
			return
		}
		if len(got) > len(want) || time.Now().After(deadline) {
			r.t.Fatalf("calls %v; want %v", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestAutoSelectsOnlyCameraAndPersists(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 2)
	r.core.Rescan()
	sn := r.core.Snapshot()
	if sn.Status != StatusConnected || sn.Selected.ID != "a" {
		t.Fatalf("%+v", sn)
	}
	if s, ok := r.store.last(); !ok || s.PreferredCamera != "a" {
		t.Fatal("preferred camera not persisted")
	}
	r.waitCalls("a", []float64{1}) // reset to normal on connect
}

func TestMultipleCamerasNeedSelection(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 2)
	r.cam.Add("b", "Cam B", 4)
	r.core.Rescan()
	if sn := r.core.Snapshot(); sn.Status != StatusSelectCamera || len(sn.Cameras) != 2 {
		t.Fatalf("%+v", sn)
	}
	r.core.SelectCamera("b")
	r.core.Rescan()
	if sn := r.core.Snapshot(); sn.Status != StatusConnected || sn.Selected.ID != "b" {
		t.Fatalf("%+v", sn)
	}
	r.waitCalls("b", []float64{1})
	r.core.Press(hotkey.CloseUp)
	r.waitCalls("b", []float64{1, 4})
}

func TestCloseUpDefaultsToMaxAndNormalTo1(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 2)
	r.core.Rescan()
	r.waitCalls("a", []float64{1})
	r.core.Press(hotkey.CloseUp)
	r.waitCalls("a", []float64{1, 2})
	r.core.Press(hotkey.Normal)
	r.waitCalls("a", []float64{1, 2, 1})
}

func TestZoomOverrides(t *testing.T) {
	s := settings.Defaults()
	s.CloseUpZoom = 3
	s.CameraZoom["b"] = settings.ZoomOverride{CloseUp: 1.5, Normal: 1.2}
	r := newRig(t, s)
	r.cam.Add("a", "Cam A", 2)
	r.cam.Add("b", "Cam B", 4)
	r.core.SelectCamera("a")
	r.core.Rescan()
	if m, _ := r.core.Target(hotkey.CloseUp); m != 2 { // 3 clamped to camera max
		t.Fatalf("a close-up %v", m)
	}
	r.core.SelectCamera("b")
	r.core.Rescan()
	if m, _ := r.core.Target(hotkey.CloseUp); m != 1.5 {
		t.Fatalf("b close-up %v", m)
	}
	if m, _ := r.core.Target(hotkey.Normal); m != 1.2 {
		t.Fatalf("b normal %v", m)
	}
}

func TestDisconnectDoesNotSwitchAndReconnects(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 2)
	r.core.Rescan()
	r.cam.Add("b", "Cam B", 4)
	r.cam.SetPresent("a", false)
	r.core.Rescan()
	sn := r.core.Snapshot()
	if sn.Status != StatusWaiting || sn.Selected.ID != "a" {
		t.Fatalf("switched or wrong status: %+v", sn)
	}
	if r.cam.OpenCount("a") != 0 || r.cam.OpenCount("b") != 0 {
		t.Fatal("handles not released / wrong camera opened")
	}
	r.core.Press(hotkey.CloseUp) // ignored, must not panic
	r.cam.SetPresent("a", true)
	r.core.Rescan()
	if sn := r.core.Snapshot(); sn.Status != StatusConnected || sn.Selected.ID != "a" {
		t.Fatalf("%+v", sn)
	}
}

func TestCommandFailureReopens(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 2)
	r.core.Rescan()
	r.waitCalls("a", []float64{1})
	r.cam.FailSet(errors.New("gone"))
	r.core.Press(hotkey.CloseUp)
	deadline := time.Now().Add(2 * time.Second)
	for r.core.Snapshot().Status != StatusError {
		if time.Now().After(deadline) {
			t.Fatal("no error status")
		}
		time.Sleep(time.Millisecond)
	}
	r.cam.FailSet(nil)
	r.core.Rescan()
	if sn := r.core.Snapshot(); sn.Status != StatusConnected {
		t.Fatalf("%+v", sn)
	}
	if r.cam.Opens() != 2 || r.cam.OpenCount("a") != 1 {
		t.Fatalf("opens %d open %d", r.cam.Opens(), r.cam.OpenCount("a"))
	}
}

func TestOpenErrorReported(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 2)
	r.cam.FailOpen(camera.ErrZoomUnsupported)
	r.core.Rescan()
	if sn := r.core.Snapshot(); sn.Status != StatusError || sn.Detail == "" {
		t.Fatalf("%+v", sn)
	}
}

func TestLaterCommandSupersedesPending(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 4)
	r.core.Rescan()
	r.waitCalls("a", []float64{1})
	gate := make(chan struct{})
	entered := make(chan struct{}, 10)
	r.cam.OnSet(func(float64) { entered <- struct{}{}; <-gate })
	r.core.Press(hotkey.CloseUp) // worker blocks inside this command
	<-entered
	r.core.Press(hotkey.Normal)  // pending...
	r.core.Press(hotkey.CloseUp) // ...superseded by this
	r.cam.OnSet(nil)
	close(gate)
	r.waitCalls("a", []float64{1, 4, 4})
}

func TestStartRegistersAndShutdownUnregisters(t *testing.T) {
	r := newRig(t, settings.Defaults())
	if err := r.core.Start(); err != nil {
		t.Fatal(err)
	}
	if k, _ := r.reg.get(hotkey.CloseUp); k != "F14" {
		t.Fatal("close-up not registered")
	}
	r.core.Shutdown()
	if _, ok := r.reg.get(hotkey.Normal); ok {
		t.Fatal("still registered")
	}
}

func TestStartReportsUnavailableKey(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.reg.taken["F13"] = true
	if err := r.core.Start(); !errors.Is(err, hotkey.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if r.core.Snapshot().HotkeyProblem == "" {
		t.Fatal("problem not surfaced")
	}
}

func TestDisableEnable(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 2)
	r.core.Start()
	r.core.Rescan()
	r.waitCalls("a", []float64{1})
	r.core.SetEnabled(false)
	if _, ok := r.reg.get(hotkey.Normal); ok {
		t.Fatal("still registered")
	}
	r.core.Press(hotkey.CloseUp)
	time.Sleep(20 * time.Millisecond)
	r.waitCalls("a", []float64{1})
	if s, _ := r.store.last(); s.HotkeysEnabled {
		t.Fatal("disabled state not persisted")
	}
	r.core.SetEnabled(true)
	if k, _ := r.reg.get(hotkey.Normal); k != "F13" {
		t.Fatal("not re-registered")
	}
	r.core.Press(hotkey.CloseUp)
	r.waitCalls("a", []float64{1, 2})
}

func TestRebindSuccessPersists(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.core.Start()
	r.core.BeginRebind(hotkey.CloseUp)
	if !r.core.Snapshot().Rebinding {
		t.Fatal("not rebinding")
	}
	if err := r.core.CompleteRebind("F20"); err != nil {
		t.Fatal(err)
	}
	if k, _ := r.reg.get(hotkey.CloseUp); k != "F20" {
		t.Fatal("not registered")
	}
	if s, _ := r.store.last(); s.CloseUpKey != "F20" || s.NormalKey != "F13" {
		t.Fatalf("persisted %+v", s)
	}
	if r.core.Snapshot().Rebinding {
		t.Fatal("still rebinding")
	}
}

func TestRebindDuplicateRejected(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.core.Start()
	r.core.BeginRebind(hotkey.CloseUp)
	if err := r.core.CompleteRebind("F13"); !errors.Is(err, hotkey.ErrDuplicate) {
		t.Fatalf("got %v", err)
	}
	if k, _ := r.reg.get(hotkey.CloseUp); k != "F14" {
		t.Fatal("previous binding lost")
	}
	if _, ok := r.store.last(); ok {
		t.Fatal("persisted after rejection")
	}
}

func TestRebindRegistrationFailureKeepsOld(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.core.Start()
	r.reg.taken["F22"] = true
	r.core.BeginRebind(hotkey.Normal)
	if err := r.core.CompleteRebind("F22"); !errors.Is(err, hotkey.ErrUnavailable) {
		t.Fatalf("got %v", err)
	}
	if k, _ := r.reg.get(hotkey.Normal); k != "F13" {
		t.Fatal("previous binding lost")
	}
	if r.core.Settings().NormalKey != "F13" {
		t.Fatal("settings changed")
	}
	if _, ok := r.store.last(); ok {
		t.Fatal("persisted after failure")
	}
}

func TestRebindCancel(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.core.Start()
	r.core.BeginRebind(hotkey.Normal)
	r.core.CancelRebind()
	if err := r.core.CompleteRebind("F20"); err == nil {
		t.Fatal("completed after cancel")
	}
	if r.core.Settings().NormalKey != "F13" {
		t.Fatal("changed")
	}
}

func TestRebindWhileDisabledTestsAvailabilityOnly(t *testing.T) {
	s := settings.Defaults()
	s.HotkeysEnabled = false
	r := newRig(t, s)
	r.core.Start()
	r.core.BeginRebind(hotkey.Normal)
	if err := r.core.CompleteRebind("F19"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.reg.get(hotkey.Normal); ok {
		t.Fatal("registered while disabled")
	}
	if r.core.Settings().NormalKey != "F19" {
		t.Fatal("not saved")
	}
}

func TestPressIgnoredWhileRebinding(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 2)
	r.core.Rescan()
	r.waitCalls("a", []float64{1})
	r.core.BeginRebind(hotkey.Normal)
	r.core.Press(hotkey.CloseUp)
	time.Sleep(20 * time.Millisecond)
	r.waitCalls("a", []float64{1})
}

func TestMonitorTriggerAndShutdownReleases(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.core.Start()
	r.cam.Add("a", "Cam A", 2)
	r.core.TriggerRescan()
	deadline := time.Now().Add(2 * time.Second)
	for r.core.Snapshot().Status != StatusConnected {
		if time.Now().After(deadline) {
			t.Fatal("not connected")
		}
		time.Sleep(time.Millisecond)
	}
	r.core.Shutdown()
	if r.cam.OpenCount("a") != 0 {
		t.Fatal("camera not released")
	}
}

func levels(sn Snapshot) map[hotkey.Action]float64 {
	m := map[hotkey.Action]float64{}
	for _, l := range sn.Zoom {
		m[l.Action] = l.Magnification
	}
	return m
}

func TestHalfwayOnlyForCamerasThatHaveIt(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "PowerConf-like", 4)
	r.cam.Add("b", "Insta360-like", 4)
	r.cam.SetHalfway("b", 2)
	r.core.Start()
	r.core.SelectCamera("a")
	r.core.Rescan()
	r.core.SyncHotkeys()
	if l := levels(r.core.Snapshot()); len(l) != 2 || l[hotkey.Normal] != 1 || l[hotkey.CloseUp] != 4 {
		t.Fatalf("camera a levels %v", l)
	}
	if _, ok := r.reg.get(hotkey.Halfway); ok {
		t.Fatal("halfway registered for a camera without it")
	}
	r.core.Press(hotkey.Halfway) // not offered: ignored
	r.waitCalls("a", []float64{1})

	r.core.SelectCamera("b")
	r.core.Rescan()
	r.core.SyncHotkeys()
	sn := r.core.Snapshot()
	if l := levels(sn); len(l) != 3 || l[hotkey.Halfway] != 2 || l[hotkey.CloseUp] != 4 {
		t.Fatalf("camera b levels %v", l)
	}
	if sn.Zoom[1].Action != hotkey.Halfway {
		t.Fatalf("order %+v", sn.Zoom)
	}
	if k, ok := r.reg.get(hotkey.Halfway); !ok || k != "F15" {
		t.Fatal("halfway not registered for camera b")
	}
	r.waitCalls("b", []float64{1})
	r.core.Press(hotkey.Halfway)
	r.waitCalls("b", []float64{1, 2})

	r.core.SelectCamera("a")
	r.core.Rescan()
	r.core.SyncHotkeys()
	if _, ok := r.reg.get(hotkey.Halfway); ok {
		t.Fatal("halfway still registered after switching back")
	}
}

func TestHalfwayOverrides(t *testing.T) {
	s := settings.Defaults()
	s.HalfwayZoom = 3
	s.CameraZoom["b"] = settings.ZoomOverride{Halfway: 1.5}
	r := newRig(t, s)
	r.cam.Add("b", "Insta360-like", 4)
	r.cam.SetHalfway("b", 2)
	r.core.Rescan()
	if l := levels(r.core.Snapshot()); l[hotkey.Halfway] != 1.5 {
		t.Fatalf("per-camera override: %v", l)
	}
	r.waitCalls("b", []float64{1})
	r.core.Press(hotkey.Halfway)
	r.waitCalls("b", []float64{1, 1.5})
}

func TestHalfwayLevelsKnownWhileDisconnected(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("b", "Insta360-like", 4)
	r.cam.SetHalfway("b", 2)
	r.core.Rescan()
	r.cam.SetPresent("b", false)
	r.core.Rescan()
	sn := r.core.Snapshot()
	if sn.Status != StatusWaiting || levels(sn)[hotkey.Halfway] != 2 {
		t.Fatalf("%v %v", sn.Status, levels(sn))
	}
}

func TestRebindHalfwayWithoutHalfwayCameraOnlyTests(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "PowerConf-like", 4)
	r.core.Start()
	r.core.Rescan()
	r.core.SyncHotkeys()
	r.core.BeginRebind(hotkey.Halfway)
	if err := r.core.CompleteRebind("F18"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.reg.get(hotkey.Halfway); ok {
		t.Fatal("halfway left registered on a camera without it")
	}
	if s, _ := r.store.last(); s.HalfwayKey != "F18" {
		t.Fatalf("not saved: %+v", s)
	}
	r.core.BeginRebind(hotkey.Normal)
	if err := r.core.CompleteRebind("F18"); !errors.Is(err, hotkey.ErrDuplicate) {
		t.Fatalf("duplicate with halfway accepted: %v", err)
	}
}

func TestDisabledRegistersNothingEvenWithHalfway(t *testing.T) {
	s := settings.Defaults()
	s.HotkeysEnabled = false
	r := newRig(t, s)
	r.cam.Add("b", "Insta360-like", 4)
	r.cam.SetHalfway("b", 2)
	r.core.Start()
	r.core.Rescan()
	r.core.SyncHotkeys()
	for _, a := range hotkey.Actions {
		if _, ok := r.reg.get(a); ok {
			t.Fatalf("%s registered while disabled", a)
		}
	}
	r.core.SetEnabled(true)
	if k, _ := r.reg.get(hotkey.Halfway); k != "F15" {
		t.Fatal("halfway not registered on enable")
	}
}

func TestMIDIBindingFreesKeyAndPresses(t *testing.T) {
	r := newRig(t, settings.Defaults())
	r.cam.Add("a", "Cam A", 4)
	r.core.Start()
	r.core.Rescan()
	r.core.SyncHotkeys()
	r.waitCalls("a", []float64{1})
	r.core.BeginRebind(hotkey.CloseUp)
	if !r.core.Rebinding() {
		t.Fatal("Rebinding false")
	}
	if err := r.core.CompleteRebind("MIDI CC 1/32"); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.reg.get(hotkey.CloseUp); ok {
		t.Fatal("close-up key still registered after binding MIDI")
	}
	if s, _ := r.store.last(); s.CloseUpKey != "MIDI CC 1/32" {
		t.Fatalf("not saved: %+v", s)
	}
	r.core.SyncHotkeys()
	if _, ok := r.reg.get(hotkey.CloseUp); ok {
		t.Fatal("sync registered a MIDI-bound action")
	}
	if !r.core.PressBinding("MIDI CC 1/32") {
		t.Fatal("binding not found")
	}
	r.waitCalls("a", []float64{1, 4})
	if r.core.PressBinding("MIDI CC 1/33") {
		t.Fatal("unbound trigger pressed something")
	}

	// Back to a key: registered again.
	r.core.BeginRebind(hotkey.CloseUp)
	if err := r.core.CompleteRebind("F14"); err != nil {
		t.Fatal(err)
	}
	if k, ok := r.reg.get(hotkey.CloseUp); !ok || k != "F14" {
		t.Fatal("key not registered after rebinding from MIDI")
	}
}

func TestMIDIPressGatedLikeKeys(t *testing.T) {
	s := settings.Defaults()
	s.CloseUpKey = "MIDI CC 1/32"
	r := newRig(t, s)
	r.cam.Add("a", "Cam A", 4)
	r.core.Rescan()
	r.waitCalls("a", []float64{1})
	r.core.SetEnabled(false)
	r.core.PressBinding("MIDI CC 1/32")
	r.core.SetEnabled(true)
	r.core.BeginRebind(hotkey.Normal)
	r.core.PressBinding("MIDI CC 1/32") // ignored while rebinding
	r.core.CancelRebind()
	time.Sleep(20 * time.Millisecond)
	r.waitCalls("a", []float64{1})
}
