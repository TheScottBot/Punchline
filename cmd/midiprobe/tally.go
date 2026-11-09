package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/thescottbot/punchline/internal/midi"
)

// control identifies one physical control as the host sees it: a source plus
// a CC or note number on a channel. Note On and Note Off count as one control.
type control struct {
	source  string
	family  string // "CC", "note", or the message kind
	channel uint8
	number  uint8
}

type counts struct {
	presses, releases int
	values            map[uint8]bool
}

// tally summarises messages per control, to answer: what does a pad send on
// press, does it send anything on release, and with which values.
type tally struct {
	order []control
	by    map[control]*counts
}

func newTally() *tally { return &tally{by: map[control]*counts{}} }

func (t *tally) add(e midi.Event) {
	m := e.Msg
	if m.Kind == midi.Other {
		return
	}
	fam := m.Kind.String()
	switch m.Kind {
	case midi.ControlChange:
		fam = "CC"
	case midi.NoteOn, midi.NoteOff:
		fam = "note"
	}
	k := control{source: e.Source.Name, family: fam, channel: m.Channel, number: m.Number}
	c, ok := t.by[k]
	if !ok {
		c = &counts{values: map[uint8]bool{}}
		t.by[k] = c
		t.order = append(t.order, k)
	}
	if m.IsPress() {
		c.presses++
	} else {
		c.releases++
	}
	c.values[m.Value] = true
}

func (t *tally) String() string {
	if len(t.order) == 0 {
		return "no channel messages received\n"
	}
	var b strings.Builder
	b.WriteString("summary per control (presses = CC > 0 or Note On > 0; releases = everything else):\n")
	for _, k := range t.order {
		c := t.by[k]
		var vs []int
		for v := range c.values {
			vs = append(vs, int(v))
		}
		sort.Ints(vs)
		num := fmt.Sprintf("#%d", k.number)
		if k.family == "note" {
			num = fmt.Sprintf("note %d", k.number)
		}
		fmt.Fprintf(&b, "  %-32s %-5s ch %-2d %-9s presses %-3d releases %-3d values %v\n",
			k.source, k.family, k.channel, num, c.presses, c.releases, vs)
	}
	return b.String()
}
