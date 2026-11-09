package desktop

// Platform-neutral rebinding flow and hotkey bookkeeping shared by the
// Windows and macOS shells. The shells supply only the native parts (how a
// key is captured, how a message is shown, how a hot key is registered), so
// the behaviour is the same on both platforms and testable without either.

import (
	"fmt"
	"log"
	"time"

	"github.com/thescottbot/punchline/internal/app"
	"github.com/thescottbot/punchline/internal/hotkey"
)

const captureTimeout = 15 * time.Second

// shellCore is the part of *app.Core the shells drive.
type shellCore interface {
	Press(hotkey.Action)
	BeginRebind(hotkey.Action)
	CancelRebind()
	CompleteRebind(hotkey.Key) error
	Snapshot() app.Snapshot
}

// rebindUI is a platform's native half of rebinding. All calls happen on
// the UI thread.
type rebindUI interface {
	// Prompt starts capturing the next key and shows text. An error means
	// capture could not start (nothing has changed).
	Prompt(title, text string) error
	// Result ends capture and shows the outcome.
	Result(title, text string, warn bool)
	// After runs f on the UI thread after d.
	After(d time.Duration, f func())
}

// rebinder runs one rebind at a time: prompt, capture, validate through the
// core (which registers before persisting), report, with Esc, a timeout and
// an external abort as ways out. Call only on the UI thread.
type rebinder struct {
	core   shellCore
	ui     rebindUI
	log    *log.Logger
	midi   bool // MIDI presses can be captured too (mentioned in the prompt)
	active bool
	action hotkey.Action
	gen    int // distinguishes rebinds so a stale timeout is ignored
}

func (r *rebinder) Active() bool { return r.active }

// Start begins rebinding a. It does nothing while a rebind is in progress.
func (r *rebinder) Start(a hotkey.Action) {
	if r.active {
		return
	}
	cur := r.core.Snapshot().Bindings.Get(a)
	what := "key"
	if r.midi {
		what = "key or MIDI pad"
	}
	if err := r.ui.Prompt("Rebind "+a.String(),
		fmt.Sprintf("Press the new %s for %s (currently %s).\nEsc cancels.", what, a, cur)); err != nil {
		r.ui.Result("Rebind failed", "Could not capture the keyboard: "+err.Error(), true)
		r.log.Printf("rebind %s: capture failed: %v", a, err)
		return
	}
	r.active, r.action = true, a
	r.gen++
	g := r.gen
	r.core.BeginRebind(a)
	r.ui.After(captureTimeout, func() {
		if r.active && r.gen == g {
			r.core.CancelRebind()
			r.finish("Rebind cancelled", "No key was pressed. Bindings are unchanged.", false)
		}
	})
}

// Captured handles the captured key: hotkey.Cancel (Esc) cancels; ok=false
// means the platform has no name for the key (code is shown).
func (r *rebinder) Captured(k hotkey.Key, ok bool, code uint32) {
	if !r.active {
		return
	}
	a := r.action
	old := r.core.Snapshot().Bindings.Get(a)
	switch {
	case ok && k == hotkey.Cancel:
		r.core.CancelRebind()
		r.finish("Rebind cancelled", fmt.Sprintf("%s stays on %s.", a, old), false)
	case !ok:
		r.core.CancelRebind()
		r.finish("Key not supported", fmt.Sprintf("That key (code 0x%02X) can't be bound. %s stays on %s.", code, a, old), true)
	default:
		if err := r.core.CompleteRebind(k); err != nil {
			r.finish("Rebind failed", fmt.Sprintf("%v.\n%s stays on %s.", err, a, old), true)
			return
		}
		r.finish("Rebound", fmt.Sprintf("%s is now %s.", a, k), false)
	}
}

// Abort cancels an in-progress rebind without a message (the capture UI went
// away: panel closed, focus lost, shutdown). It reports whether one was
// active; the caller tears down its own UI.
func (r *rebinder) Abort(why string) bool {
	if !r.active {
		return false
	}
	r.active = false
	r.core.CancelRebind()
	r.log.Printf("rebind %s: cancelled (%s)", r.action, why)
	return true
}

func (r *rebinder) finish(title, text string, warn bool) {
	r.active = false
	r.log.Printf("rebind %s: %s: %s", r.action, title, text)
	r.ui.Result(title, text, warn)
}

// nativeHotkeys registers one OS hot key per call. register returns a handle
// passed back to unregister, or an error wrapping hotkey.ErrUnsupportedKey or
// hotkey.ErrUnavailable.
type nativeHotkeys interface {
	register(k hotkey.Key, id uint32) (handle any, err error)
	unregister(handle any)
}

type hotkeyEntry struct {
	id     uint32
	handle any
	action hotkey.Action
	key    hotkey.Key
}

// hotkeyTable implements app.Registrar over a platform's native hot keys:
// a fresh ID per registration so the replacement is registered before the old
// one is released, and on failure the old one stays. Use on the UI thread.
type hotkeyTable struct {
	native nativeHotkeys
	maxID  uint32
	next   uint32
	regs   map[hotkey.Action]hotkeyEntry
	byID   map[uint32]hotkeyEntry
}

func newHotkeyTable(n nativeHotkeys, maxID uint32) *hotkeyTable {
	return &hotkeyTable{native: n, maxID: maxID, regs: map[hotkey.Action]hotkeyEntry{}, byID: map[uint32]hotkeyEntry{}}
}

func (t *hotkeyTable) nextID() uint32 {
	for {
		t.next++
		if t.next == 0 || t.next > t.maxID {
			t.next = 1
		}
		if _, used := t.byID[t.next]; !used {
			return t.next
		}
	}
}

func (t *hotkeyTable) Register(a hotkey.Action, k hotkey.Key) error {
	if old, ok := t.regs[a]; ok && old.key == k {
		return nil
	}
	id := t.nextID()
	h, err := t.native.register(k, id)
	if err != nil {
		return err
	}
	if old, ok := t.regs[a]; ok {
		t.native.unregister(old.handle)
		delete(t.byID, old.id)
	}
	e := hotkeyEntry{id: id, handle: h, action: a, key: k}
	t.regs[a], t.byID[id] = e, e
	return nil
}

func (t *hotkeyTable) Unregister(a hotkey.Action) {
	if old, ok := t.regs[a]; ok {
		t.native.unregister(old.handle)
		delete(t.byID, old.id)
		delete(t.regs, a)
	}
}

// lookup maps a fired hot key ID to its action and key.
func (t *hotkeyTable) lookup(id uint32) (hotkey.Action, hotkey.Key, bool) {
	e, ok := t.byID[id]
	return e.action, e.key, ok
}

// dispatchHotkey handles a fired hot key: it zooms, or, while rebinding,
// counts as the captured key (a bound key reaches its hot key before any
// capture UI sees it).
func dispatchHotkey(t *hotkeyTable, rb *rebinder, core shellCore, id uint32) {
	a, k, ok := t.lookup(id)
	if !ok {
		return
	}
	if rb.Active() {
		rb.Captured(k, true, 0)
		return
	}
	core.Press(a)
}
