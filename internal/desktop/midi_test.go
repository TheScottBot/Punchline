package desktop

import (
	"strings"
	"testing"
	"time"

	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/midi"
)

func midiEv(s, d1, d2 byte) midi.Event { return midi.Event{Msg: midi.Decode(s, d1, d2)} }

func TestMIDIBindingAndCapturable(t *testing.T) {
	cases := []struct {
		in      midi.Event
		key     hotkey.Key
		ok, cap bool
	}{
		{midiEv(0xb0, 32, 127), "MIDI CC 1/32", true, true}, // RØDECaster pad press
		{midiEv(0xb0, 32, 0), "MIDI CC 1/32", true, false},  // its release
		{midiEv(0xb5, 15, 4), "MIDI CC 6/15", true, false},  // the knob seen in the probe
		{midiEv(0x99, 36, 100), "MIDI Note 10/36", true, true},
		{midiEv(0x89, 36, 0), "MIDI Note 10/36", true, false},
		{midiEv(0xc0, 1, 0), "", false, false},
		{midiEv(0xf8, 0, 0), "", false, false},
	}
	for _, c := range cases {
		k, ok := midiBinding(c.in.Msg)
		if k != c.key || ok != c.ok || midiCapturable(c.in.Msg) != c.cap {
			t.Errorf("%v: got %q %v cap %v", c.in.Msg, k, ok, midiCapturable(c.in.Msg))
		}
	}
}

// The router against a real core: presses zoom, a rebind captures a pad but
// not a knob, releases do nothing.
func TestMIDIRouter(t *testing.T) {
	r := newShellRig(t)
	r.rb.midi = true
	r.cam.Add("a", "Cam", 4)
	r.core.Rescan()
	var ui []func()
	router := midiRouter{
		core:   r.core,
		onUI:   func(f func()) { ui = append(ui, f) },
		rebind: func(k hotkey.Key) { r.rb.Captured(k, true, 0) },
	}
	runUI := func() {
		for _, f := range ui {
			f()
		}
		ui = nil
	}

	// Bind Close-Up to a pad. A knob turned while the prompt is up is ignored.
	r.rb.Start(hotkey.CloseUp)
	if e := r.ui.last(); !strings.Contains(e.text, "key or MIDI pad") {
		t.Fatalf("prompt %q", e.text)
	}
	router.Event(midiEv(0xb5, 15, 4))
	router.Event(midiEv(0xb0, 32, 0)) // a release
	if len(ui) != 0 {
		t.Fatal("knob or release handed to the rebind")
	}
	router.Event(midiEv(0xb0, 32, 127))
	runUI()
	if e := r.ui.last(); e.title != "Rebound" || e.text != "Close-Up is now MIDI CC 1/32." {
		t.Fatalf("%+v", e)
	}
	if _, ok := r.table.regs[hotkey.CloseUp]; ok {
		t.Fatal("close-up hot key still registered")
	}

	// Now the pad zooms, straight through the core (no UI hop).
	waitFor(t, func() bool { return len(r.cam.Calls("a")) >= 1 })
	before := len(r.cam.Calls("a"))
	router.Event(midiEv(0xb0, 32, 127))
	if len(ui) != 0 {
		t.Fatal("press went through the UI thread")
	}
	waitFor(t, func() bool { c := r.cam.Calls("a"); return len(c) == before+1 && c[len(c)-1] == 4 })
	router.Event(midiEv(0xb0, 32, 0))   // release: nothing
	router.Event(midiEv(0xb0, 33, 127)) // unbound pad: nothing
	time.Sleep(20 * time.Millisecond)
	if len(r.cam.Calls("a")) != before+1 {
		t.Fatal("release or unbound pad zoomed")
	}

	// Binding a second action to the same pad is refused.
	r.rb.Start(hotkey.Normal)
	router.Event(midiEv(0xb0, 32, 127))
	runUI()
	if e := r.ui.last(); e.title != "Rebind failed" {
		t.Fatalf("%+v", e)
	}
}

func TestMIDILine(t *testing.T) {
	keys := hotkey.DefaultBindings()
	pad := keys.With(hotkey.CloseUp, "MIDI CC 1/32")
	cases := []struct {
		b    hotkey.Bindings
		m    MIDIState
		want string
	}{
		{keys, MIDIState{}, ""},
		{pad, MIDIState{}, "MIDI: not available on this platform yet"},
		{keys, MIDIState{Supported: true}, ""},
		{pad, MIDIState{Supported: true}, "MIDI: no input connected"},
		{keys, MIDIState{Supported: true, Sources: []string{"RODECaster Pro II"}}, "MIDI: RODECaster Pro II"},
		{pad, MIDIState{Supported: true, Err: "OSStatus -1"}, "MIDI error: OSStatus -1"},
	}
	for _, c := range cases {
		if got := midiLine(c.b, c.m); got != c.want {
			t.Errorf("%+v %+v: got %q want %q", c.b, c.m, got, c.want)
		}
	}
	sn := base()
	sn.Bindings = pad
	items := BuildMenu(sn, StartupState{}, PermissionState{}, MIDIState{Supported: true, Sources: []string{"RODECaster Pro II"}})
	if items[2].Text != "MIDI: RODECaster Pro II" {
		t.Errorf("menu line %q", items[2].Text)
	}
	if rb := find(items, CmdRebind); !strings.Contains(rb[len(rb)-1].Text, "(MIDI CC 1/32)") {
		t.Errorf("%+v", rb)
	}
}
