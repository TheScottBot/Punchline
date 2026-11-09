package main

import (
	"strings"
	"testing"

	"github.com/thescottbot/punchline/internal/midi"
)

func ev(src string, s, d1, d2 byte) midi.Event {
	return midi.Event{Source: midi.Source{Name: src}, Msg: midi.Decode(s, d1, d2)}
}

func TestTally(t *testing.T) {
	tl := newTally()
	tl.add(ev("Pads", 0xb0, 3, 127)) // pad press
	tl.add(ev("Pads", 0xb0, 3, 0))   // pad release
	tl.add(ev("Pads", 0xb0, 3, 127))
	tl.add(ev("Pads", 0x99, 36, 100)) // note on
	tl.add(ev("Pads", 0x89, 36, 0))   // note off: same control
	tl.add(ev("Pads", 0x99, 36, 0))   // velocity-0 note on: release
	tl.add(ev("Pads", 0xf8, 0, 0))    // clock: ignored
	out := tl.String()
	if len(tl.order) != 2 {
		t.Fatalf("controls %v", tl.order)
	}
	if c := tl.by[tl.order[0]]; c.presses != 2 || c.releases != 1 || len(c.values) != 2 {
		t.Fatalf("CC counts %+v", c)
	}
	if c := tl.by[tl.order[1]]; c.presses != 1 || c.releases != 2 {
		t.Fatalf("note counts %+v", c)
	}
	for _, want := range []string{"CC    ch 1  #3", "values [0 127]", "note  ch 10 note 36"} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
	if newTally().String() != "no channel messages received\n" {
		t.Error("empty summary")
	}
}
