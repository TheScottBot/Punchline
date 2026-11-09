// Package midi receives MIDI input through each operating system's native
// MIDI API (macOS CoreMIDI; Windows to follow) and decodes channel messages.
// Like internal/uvc it is an OS-facing package; the decoding in this file is
// pure Go.
package midi

import (
	"errors"
	"fmt"
	"time"
)

// Kind is a channel message type.
type Kind uint8

const (
	Other Kind = iota // system, realtime or anything not decoded
	NoteOff
	NoteOn
	PolyPressure
	ControlChange
	ProgramChange
	ChannelPressure
	PitchBend
)

func (k Kind) String() string {
	return [...]string{"other", "note off", "note on", "poly pressure", "CC", "program", "channel pressure", "pitch bend"}[k]
}

// Message is one decoded short MIDI message.
type Message struct {
	Kind    Kind
	Channel uint8 // 1-16 for channel messages, 0 otherwise
	Number  uint8 // note, controller or program number
	Value   uint8 // velocity, controller value, pressure; pitch bend MSB
	Raw     [3]byte
}

// Decode decodes a short message from its status byte and up to two data
// bytes (as CoreMIDI's MIDI 1.0 UMP words and winmm's packed messages
// deliver them: always complete, no running status).
func Decode(status, d1, d2 byte) Message {
	m := Message{Raw: [3]byte{status, d1 & 0x7f, d2 & 0x7f}}
	if status < 0x80 || status >= 0xf0 {
		return m
	}
	m.Channel = status&0x0f + 1
	m.Number, m.Value = d1&0x7f, d2&0x7f
	switch status & 0xf0 {
	case 0x80:
		m.Kind = NoteOff
	case 0x90:
		m.Kind = NoteOn
	case 0xa0:
		m.Kind = PolyPressure
	case 0xb0:
		m.Kind = ControlChange
	case 0xc0:
		m.Kind, m.Value = ProgramChange, 0
	case 0xd0:
		m.Kind, m.Number, m.Value = ChannelPressure, 0, d1&0x7f
	case 0xe0:
		m.Kind, m.Number, m.Value = PitchBend, 0, d2&0x7f
	}
	return m
}

// IsPress reports whether the message is a button press: a CC with a
// non-zero value or a Note On with non-zero velocity. CC value 0, Note Off
// and velocity-0 Note On are releases.
func (m Message) IsPress() bool {
	switch m.Kind {
	case ControlChange, NoteOn:
		return m.Value > 0
	}
	return false
}

// IsRealtime reports clock, start/stop, active sensing and similar
// single-byte system messages, which controllers can send continuously.
func (m Message) IsRealtime() bool { return m.Raw[0] >= 0xf8 }

func (m Message) String() string {
	switch m.Kind {
	case NoteOn, NoteOff, PolyPressure:
		return fmt.Sprintf("%s ch %d note %d value %d", m.Kind, m.Channel, m.Number, m.Value)
	case ControlChange:
		return fmt.Sprintf("CC ch %d #%d value %d", m.Channel, m.Number, m.Value)
	case ProgramChange:
		return fmt.Sprintf("program ch %d #%d", m.Channel, m.Number)
	case ChannelPressure, PitchBend:
		return fmt.Sprintf("%s ch %d value %d", m.Kind, m.Channel, m.Value)
	}
	return fmt.Sprintf("system %02X", m.Raw[0])
}

// Source is a MIDI input endpoint.
type Source struct {
	ID   int32  // stable per device where the OS provides it (CoreMIDI unique ID)
	Name string // display name, e.g. "RØDECaster Pro II MIDI Function"
}

// Event is a message received from a source.
type Event struct {
	Source Source
	Msg    Message
	At     time.Time
}

var ErrUnsupportedPlatform = errors.New("midi: not implemented on this platform")
