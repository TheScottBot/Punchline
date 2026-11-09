package desktop

// Shared MIDI handling: which messages are bindings, which may be captured
// by a rebind, and where a press goes. The platform shell owns the MIDI
// client and feeds every event to midiRouter.Event.

import (
	"strings"

	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/midi"
)

// midiBinding is the binding a message belongs to: its CC, or its note for
// Note On and Note Off alike.
func midiBinding(m midi.Message) (hotkey.Key, bool) {
	switch m.Kind {
	case midi.ControlChange:
		return hotkey.MIDITrigger{Channel: m.Channel, Number: m.Number}.Key(), true
	case midi.NoteOn, midi.NoteOff:
		return hotkey.MIDITrigger{Note: true, Channel: m.Channel, Number: m.Number}.Key(), true
	}
	return "", false
}

// midiCapturable reports whether a rebind may take this message: a pad or
// button going fully on (CC value 127, or Note On). Knobs, faders and
// encoders send in-between CC values and are ignored, so turning one while
// the rebind prompt is up doesn't bind it.
func midiCapturable(m midi.Message) bool {
	switch m.Kind {
	case midi.ControlChange:
		return m.Value == 127
	case midi.NoteOn:
		return m.Value > 0
	}
	return false
}

type midiCore interface {
	Rebinding() bool
	PressBinding(hotkey.Key) bool
}

// midiRouter routes MIDI events. Event runs on the MIDI thread: a press zooms
// straight through the core (thread-safe, never blocks); during a rebind a
// capturable press is handed to the UI thread for the rebinder.
type midiRouter struct {
	core   midiCore
	onUI   func(func())     // run on the UI thread
	rebind func(hotkey.Key) // called on the UI thread
}

func (r midiRouter) Event(e midi.Event) {
	if !e.Msg.IsPress() {
		return
	}
	k, ok := midiBinding(e.Msg)
	if !ok {
		return
	}
	if r.core.Rebinding() {
		if midiCapturable(e.Msg) {
			r.onUI(func() { r.rebind(k) })
		}
		return
	}
	r.core.PressBinding(k)
}

// MIDIState describes MIDI input for the menu.
type MIDIState struct {
	Supported bool     // this platform has a MIDI backend
	Sources   []string // connected input names
	Err       string   // the backend failed to start
}

// midiLine is the menu's MIDI status line, or "" when there's nothing worth
// saying (no inputs and nothing bound to MIDI).
func midiLine(b hotkey.Bindings, m MIDIState) string {
	usesMIDI := false
	for _, a := range hotkey.Actions {
		usesMIDI = usesMIDI || b.Get(a).IsMIDI()
	}
	switch {
	case !m.Supported:
		if usesMIDI {
			return "MIDI: not available on this platform yet"
		}
	case m.Err != "":
		return "MIDI error: " + truncate(m.Err, 60)
	case len(m.Sources) > 0:
		return "MIDI: " + truncate(strings.Join(m.Sources, ", "), 70)
	case usesMIDI:
		return "MIDI: no input connected"
	}
	return ""
}
