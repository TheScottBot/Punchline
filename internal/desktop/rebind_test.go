package desktop

import (
	"errors"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/thescottbot/punchline/internal/app"
	"github.com/thescottbot/punchline/internal/camera"
	"github.com/thescottbot/punchline/internal/camera/mock"
	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/settings"
)

// ---- fakes ----------------------------------------------------------------

type uiEvent struct {
	kind, title, text string
	warn              bool
}

type fakeUI struct {
	events    []uiEvent
	timers    []func()
	promptErr error
}

func (u *fakeUI) Prompt(title, text string) error {
	u.events = append(u.events, uiEvent{kind: "prompt", title: title, text: text})
	return u.promptErr
}

func (u *fakeUI) Result(title, text string, warn bool) {
	u.events = append(u.events, uiEvent{kind: "result", title: title, text: text, warn: warn})
}

func (u *fakeUI) After(d time.Duration, f func()) {
	if d != captureTimeout {
		panic("unexpected timer")
	}
	u.timers = append(u.timers, f)
}

func (u *fakeUI) last() uiEvent { return u.events[len(u.events)-1] }

// fakeNative records native hot key calls in order.
type fakeNative struct {
	calls []string
	taken map[hotkey.Key]bool
	live  map[uint32]hotkey.Key
}

func newFakeNative() *fakeNative {
	return &fakeNative{taken: map[hotkey.Key]bool{}, live: map[uint32]hotkey.Key{}}
}

func (n *fakeNative) register(k hotkey.Key, id uint32) (any, error) {
	n.calls = append(n.calls, "reg "+string(k))
	if n.taken[k] {
		return nil, hotkey.ErrUnavailable
	}
	for _, held := range n.live {
		if held == k {
			return nil, hotkey.ErrUnavailable // the OS refuses a second registration
		}
	}
	n.live[id] = k
	return id, nil
}

func (n *fakeNative) unregister(h any) {
	id := h.(uint32)
	n.calls = append(n.calls, "unreg "+string(n.live[id]))
	delete(n.live, id)
}

type storeFunc func(settings.Settings) error

func (f storeFunc) Save(s settings.Settings) error { return f(s) }

// rig wires a real core, the shared table as its registrar, and the rebinder.
type shellRig struct {
	t      *testing.T
	core   *app.Core
	cam    *mock.Adapter
	native *fakeNative
	table  *hotkeyTable
	ui     *fakeUI
	rb     *rebinder
	saved  []settings.Settings
}

func newShellRig(t *testing.T) *shellRig {
	r := &shellRig{t: t, cam: mock.New("mock"), native: newFakeNative(), ui: &fakeUI{}}
	r.table = newHotkeyTable(r.native, 1000)
	r.core = app.New(app.Options{
		Adapters:  []camera.Adapter{r.cam},
		Registrar: r.table,
		Store:     storeFunc(func(s settings.Settings) error { r.saved = append(r.saved, s.Clone()); return nil }),
		Settings:  settings.Defaults(),
	})
	t.Cleanup(r.core.Shutdown)
	r.rb = &rebinder{core: r.core, ui: r.ui, log: log.New(io.Discard, "", 0)}
	if err := r.core.Start(); err != nil {
		t.Fatal(err)
	}
	return r
}

func (r *shellRig) binding(a hotkey.Action) hotkey.Key { return r.core.Snapshot().Bindings.Get(a) }

// ---- rebinder -------------------------------------------------------------

func TestRebindSuccessRegistersThenPersists(t *testing.T) {
	r := newShellRig(t)
	r.rb.Start(hotkey.CloseUp)
	if !r.rb.Active() || !r.core.Snapshot().Rebinding {
		t.Fatal("not rebinding")
	}
	if e := r.ui.last(); e.kind != "prompt" || e.title != "Rebind Close-Up" || !strings.Contains(e.text, "currently F14") {
		t.Fatalf("%+v", e)
	}
	r.rb.Captured("F18", true, 0)
	if e := r.ui.last(); e.title != "Rebound" || e.text != "Close-Up is now F18." || e.warn {
		t.Fatalf("%+v", e)
	}
	if r.rb.Active() || r.core.Snapshot().Rebinding || r.binding(hotkey.CloseUp) != "F18" {
		t.Fatal("state after success")
	}
	if len(r.saved) == 0 || r.saved[len(r.saved)-1].CloseUpKey != "F18" {
		t.Fatal("not persisted")
	}
	// The new key was registered before the old one was released.
	c := strings.Join(r.native.calls, ",")
	if !strings.HasSuffix(c, "reg F18,unreg F14") {
		t.Fatalf("calls %s", c)
	}
}

func TestRebindEscapeCancels(t *testing.T) {
	r := newShellRig(t)
	r.rb.Start(hotkey.Normal)
	r.rb.Captured(hotkey.Cancel, true, 0x1B)
	if e := r.ui.last(); e.title != "Rebind cancelled" || e.text != "Normal Zoom stays on F13." {
		t.Fatalf("%+v", e)
	}
	if r.rb.Active() || r.core.Snapshot().Rebinding || r.binding(hotkey.Normal) != "F13" {
		t.Fatal("state after cancel")
	}
}

func TestRebindUnknownKey(t *testing.T) {
	r := newShellRig(t)
	r.rb.Start(hotkey.Normal)
	r.rb.Captured("", false, 0xE8)
	if e := r.ui.last(); e.title != "Key not supported" || !e.warn || !strings.Contains(e.text, "0xE8") {
		t.Fatalf("%+v", e)
	}
	if r.core.Snapshot().Rebinding || r.binding(hotkey.Normal) != "F13" {
		t.Fatal("state")
	}
}

func TestRebindDuplicateRefused(t *testing.T) {
	r := newShellRig(t)
	r.rb.Start(hotkey.Normal)
	r.rb.Captured("F14", true, 0)
	e := r.ui.last()
	if e.title != "Rebind failed" || !e.warn || !strings.Contains(e.text, "Normal Zoom stays on F13") {
		t.Fatalf("%+v", e)
	}
	if r.binding(hotkey.Normal) != "F13" || r.binding(hotkey.CloseUp) != "F14" {
		t.Fatal("bindings changed")
	}
}

func TestRebindKeyHeldElsewhereKeepsOldBinding(t *testing.T) {
	r := newShellRig(t)
	r.native.taken["F19"] = true
	saves := len(r.saved)
	r.rb.Start(hotkey.CloseUp)
	r.rb.Captured("F19", true, 0)
	if e := r.ui.last(); e.title != "Rebind failed" || !strings.Contains(e.text, "stays on F14") {
		t.Fatalf("%+v", e)
	}
	if r.binding(hotkey.CloseUp) != "F14" || len(r.saved) != saves {
		t.Fatal("binding changed or persisted")
	}
	if a, k, ok := r.table.lookup(r.table.regs[hotkey.CloseUp].id); !ok || a != hotkey.CloseUp || k != "F14" {
		t.Fatal("old hot key no longer registered")
	}
}

func TestRebindTimeoutAndStaleTimer(t *testing.T) {
	r := newShellRig(t)
	r.rb.Start(hotkey.Normal)
	first := r.ui.timers[0]
	r.rb.Captured("F20", true, 0) // finishes before the timeout
	r.rb.Start(hotkey.CloseUp)    // a second rebind
	first()                       // the first rebind's timer fires late: ignored
	if !r.rb.Active() {
		t.Fatal("stale timer cancelled the second rebind")
	}
	r.ui.timers[1]()
	if e := r.ui.last(); e.title != "Rebind cancelled" || !strings.Contains(e.text, "No key was pressed") {
		t.Fatalf("%+v", e)
	}
	if r.rb.Active() || r.core.Snapshot().Rebinding || r.binding(hotkey.CloseUp) != "F14" {
		t.Fatal("state after timeout")
	}
}

func TestRebindPromptFailure(t *testing.T) {
	r := newShellRig(t)
	r.ui.promptErr = errors.New("hook refused")
	r.rb.Start(hotkey.Normal)
	if r.rb.Active() || r.core.Snapshot().Rebinding || len(r.ui.timers) != 0 {
		t.Fatal("rebind started despite prompt failure")
	}
	if e := r.ui.last(); e.title != "Rebind failed" || !strings.Contains(e.text, "hook refused") {
		t.Fatalf("%+v", e)
	}
}

func TestRebindStartWhileActiveAndAbort(t *testing.T) {
	r := newShellRig(t)
	r.rb.Start(hotkey.Normal)
	r.rb.Start(hotkey.CloseUp) // ignored
	if r.rb.action != hotkey.Normal || len(r.ui.events) != 1 {
		t.Fatal("second Start not ignored")
	}
	if !r.rb.Abort("panel closed") || r.rb.Active() || r.core.Snapshot().Rebinding {
		t.Fatal("abort")
	}
	if r.rb.Abort("again") {
		t.Fatal("abort reported active twice")
	}
	r.rb.Captured("F20", true, 0) // after abort: ignored
	if r.binding(hotkey.Normal) != "F13" {
		t.Fatal("key after abort applied")
	}
}

// ---- dispatch -------------------------------------------------------------

func TestDispatchPressesOrFeedsTheRebind(t *testing.T) {
	r := newShellRig(t)
	r.cam.Add("a", "Cam", 4)
	r.core.Rescan()
	id := r.table.regs[hotkey.CloseUp].id
	dispatchHotkey(r.table, r.rb, r.core, id)
	// The connect-time reset to 1x may be superseded by the press; either way
	// the last command is the close-up.
	waitFor(t, func() bool { c := r.cam.Calls("a"); return len(c) > 0 && c[len(c)-1] == 4 })
	before := len(r.cam.Calls("a"))

	// While rebinding Normal, the hot key for F14 arrives first: it counts as
	// the captured key, so the duplicate is refused rather than zooming.
	r.rb.Start(hotkey.Normal)
	dispatchHotkey(r.table, r.rb, r.core, id)
	if e := r.ui.last(); e.title != "Rebind failed" {
		t.Fatalf("%+v", e)
	}
	time.Sleep(20 * time.Millisecond)
	if len(r.cam.Calls("a")) != before {
		t.Fatal("zoomed during rebind")
	}
	dispatchHotkey(r.table, r.rb, r.core, 999) // unknown ID: nothing
}

func waitFor(t *testing.T, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

// ---- hotkey table ---------------------------------------------------------

func TestTableReplacesOnlyAfterSuccess(t *testing.T) {
	n := newFakeNative()
	tb := newHotkeyTable(n, 1000)
	if err := tb.Register(hotkey.Normal, "F13"); err != nil {
		t.Fatal(err)
	}
	if err := tb.Register(hotkey.Normal, "F13"); err != nil || len(n.calls) != 1 {
		t.Fatalf("same key re-registered: %v %v", err, n.calls)
	}
	n.taken["F20"] = true
	if err := tb.Register(hotkey.Normal, "F20"); !errors.Is(err, hotkey.ErrUnavailable) {
		t.Fatal(err)
	}
	if a, k, ok := tb.lookup(tb.regs[hotkey.Normal].id); !ok || a != hotkey.Normal || k != "F13" {
		t.Fatal("old registration lost on failure")
	}
	old := tb.regs[hotkey.Normal].id
	if err := tb.Register(hotkey.Normal, "F16"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := tb.lookup(old); ok {
		t.Fatal("old ID still resolves")
	}
	want := "reg F13,reg F20,reg F16,unreg F13"
	if got := strings.Join(n.calls, ","); got != want {
		t.Fatalf("calls %s; want %s", got, want)
	}
	tb.Unregister(hotkey.Normal)
	tb.Unregister(hotkey.Normal) // no-op
	if len(n.live) != 0 || len(tb.byID) != 0 {
		t.Fatal("not released")
	}
}

func TestTableIDsWrapAndSkipLiveOnes(t *testing.T) {
	n := newFakeNative()
	tb := newHotkeyTable(n, 3)
	tb.Register(hotkey.Normal, "A")  // id 1, stays
	tb.Register(hotkey.CloseUp, "B") // id 2
	tb.Register(hotkey.CloseUp, "C") // id 3, releases 2
	tb.Register(hotkey.CloseUp, "D") // wraps: 1 is live, takes 2
	if tb.regs[hotkey.Normal].id != 1 || tb.regs[hotkey.CloseUp].id != 2 {
		t.Fatalf("%+v", tb.regs)
	}
	if a, _, _ := tb.lookup(1); a != hotkey.Normal {
		t.Fatal("live ID reused")
	}
}
