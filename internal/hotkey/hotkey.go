// Package hotkey models the zoom actions, the platform-neutral key names
// they can be bound to, and binding validation. It has no OS dependencies;
// each desktop implementation maps these names onto native key codes.
package hotkey

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Action is a zoom action triggered by a hotkey.
type Action int

const (
	Normal  Action = iota
	CloseUp        // the camera's maximum by default
	Halfway        // an intermediate level, only on cameras whose model defines one
)

// Actions lists every action, in menu order (increasing zoom).
var Actions = []Action{Normal, Halfway, CloseUp}

func (a Action) String() string {
	switch a {
	case Normal:
		return "Normal Zoom"
	case Halfway:
		return "Halfway"
	case CloseUp:
		return "Close-Up"
	}
	return fmt.Sprintf("Action(%d)", int(a))
}

// Key is a binding: a canonical, platform-neutral key name such as "F13",
// or a MIDI trigger such as "MIDI CC 1/32" (see MIDI). An action is bound to
// exactly one of the two.
type Key string

// Cancel is the key that cancels rebinding. It can never be bound.
const Cancel Key = "Escape"

var (
	ErrUnsupportedKey = errors.New("key is not supported")
	ErrDuplicate      = errors.New("key is already bound to the other action")
	ErrUnavailable    = errors.New("key is unavailable")
)

var supported = buildSupported()

var lookup = func() map[string]Key {
	m := make(map[string]Key, len(supported))
	for _, k := range supported {
		m[strings.ToLower(string(k))] = k
	}
	return m
}()

func buildSupported() []Key {
	var ks []Key
	for i := 1; i <= 24; i++ {
		ks = append(ks, Key(fmt.Sprintf("F%d", i)))
	}
	for c := 'A'; c <= 'Z'; c++ {
		ks = append(ks, Key(string(c)))
	}
	for c := '0'; c <= '9'; c++ {
		ks = append(ks, Key(string(c)))
	}
	for c := '0'; c <= '9'; c++ {
		ks = append(ks, Key("Num"+string(c)))
	}
	ks = append(ks,
		"NumMultiply", "NumAdd", "NumSubtract", "NumDecimal", "NumDivide",
		"Space", "Tab", "Enter", "Backspace",
		"Insert", "Delete", "Home", "End", "PageUp", "PageDown",
		"Up", "Down", "Left", "Right",
		"Pause", "ScrollLock", "PrintScreen", "Menu",
		"Minus", "Equals", "LeftBracket", "RightBracket", "Backslash",
		"Semicolon", "Quote", "Backquote", "Comma", "Period", "Slash",
		"VolumeMute", "VolumeDown", "VolumeUp",
		"MediaNext", "MediaPrevious", "MediaStop", "MediaPlayPause",
	)
	return ks
}

// Supported returns every bindable keyboard key, in a stable order.
func Supported() []Key { return append([]Key(nil), supported...) }

// Parse returns the canonical binding for a case-insensitive key name or
// MIDI trigger ("midi cc 1/32", "MIDI Note 10 36").
func Parse(name string) (Key, bool) {
	if t, ok := parseMIDI(name); ok {
		return t.Key(), true
	}
	k, ok := lookup[strings.ToLower(strings.TrimSpace(name))]
	return k, ok
}

// IsSupported reports whether k is a canonical binding: a keyboard key or a
// MIDI trigger.
func IsSupported(k Key) bool {
	if t, ok := parseMIDI(string(k)); ok {
		return t.Key() == k
	}
	c, ok := lookup[strings.ToLower(string(k))]
	return ok && c == k
}

// MIDITrigger is a MIDI message that triggers an action when pressed: a
// Control Change (fires on a non-zero value) or a Note (fires on Note On
// with non-zero velocity).
type MIDITrigger struct {
	Note    bool  // false: Control Change
	Channel uint8 // 1-16
	Number  uint8 // controller or note number, 0-127
}

// Key returns the trigger's canonical binding, e.g. "MIDI CC 1/32".
func (t MIDITrigger) Key() Key {
	kind := "CC"
	if t.Note {
		kind = "Note"
	}
	return Key(fmt.Sprintf("MIDI %s %d/%d", kind, t.Channel, t.Number))
}

// MIDI returns the trigger a binding names, if it is a MIDI binding.
func (k Key) MIDI() (MIDITrigger, bool) {
	if t, ok := parseMIDI(string(k)); ok && t.Key() == k {
		return t, true
	}
	return MIDITrigger{}, false
}

// IsMIDI reports whether k is a MIDI binding (not a keyboard key).
func (k Key) IsMIDI() bool { _, ok := k.MIDI(); return ok }

func parseMIDI(s string) (MIDITrigger, bool) {
	f := strings.Fields(strings.ToLower(strings.ReplaceAll(s, "/", " ")))
	if len(f) != 4 || f[0] != "midi" || (f[1] != "cc" && f[1] != "note") {
		return MIDITrigger{}, false
	}
	ch, err1 := strconv.Atoi(f[2])
	n, err2 := strconv.Atoi(f[3])
	if err1 != nil || err2 != nil || ch < 1 || ch > 16 || n < 0 || n > 127 {
		return MIDITrigger{}, false
	}
	return MIDITrigger{Note: f[1] == "note", Channel: uint8(ch), Number: uint8(n)}, true
}

// Bindings holds the key for each action.
type Bindings struct {
	Normal  Key
	Halfway Key
	CloseUp Key
}

// DefaultBindings are F13 for normal framing, F15 for halfway and F14 for
// close-up (F14 predates the halfway level).
func DefaultBindings() Bindings { return Bindings{Normal: "F13", Halfway: "F15", CloseUp: "F14"} }

// Get returns the key bound to a.
func (b Bindings) Get(a Action) Key {
	switch a {
	case CloseUp:
		return b.CloseUp
	case Halfway:
		return b.Halfway
	}
	return b.Normal
}

// With returns a copy of b with a bound to k.
func (b Bindings) With(a Action, k Key) Bindings {
	switch a {
	case CloseUp:
		b.CloseUp = k
	case Halfway:
		b.Halfway = k
	default:
		b.Normal = k
	}
	return b
}

// Validate checks every key is supported and no key is bound twice.
func (b Bindings) Validate() error {
	seen := map[Key]Action{}
	for _, a := range Actions {
		k := b.Get(a)
		if !IsSupported(k) {
			return fmt.Errorf("%s: %q: %w", a, k, ErrUnsupportedKey)
		}
		if o, dup := seen[k]; dup {
			return fmt.Errorf("%s is bound to both %s and %s: %w", k, o, a, ErrDuplicate)
		}
		seen[k] = a
	}
	return nil
}

// CheckRebind validates binding a to k given the current bindings.
func CheckRebind(b Bindings, a Action, k Key) error {
	if k == Cancel || !IsSupported(k) {
		return fmt.Errorf("%s: %w", k, ErrUnsupportedKey)
	}
	for _, o := range Actions {
		if o != a && b.Get(o) == k {
			return fmt.Errorf("%s is already bound to %s: %w", k, o, ErrDuplicate)
		}
	}
	return nil
}
