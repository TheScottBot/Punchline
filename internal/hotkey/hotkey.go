// Package hotkey models the two zoom actions, the platform-neutral key names
// they can be bound to, and binding validation. It has no OS dependencies;
// each desktop implementation maps these names onto native key codes.
package hotkey

import (
	"errors"
	"fmt"
	"strings"
)

// Action is a zoom action triggered by a hotkey.
type Action int

const (
	Normal Action = iota
	CloseUp
)

// Actions lists every action, in menu order.
var Actions = []Action{Normal, CloseUp}

func (a Action) String() string {
	switch a {
	case Normal:
		return "Normal Zoom"
	case CloseUp:
		return "Close-Up"
	}
	return fmt.Sprintf("Action(%d)", int(a))
}

// Other returns the other action.
func (a Action) Other() Action {
	if a == Normal {
		return CloseUp
	}
	return Normal
}

// Key is a canonical, platform-neutral key name such as "F13".
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

// Supported returns every bindable key, in a stable order.
func Supported() []Key { return append([]Key(nil), supported...) }

// Parse returns the canonical key for a case-insensitive name.
func Parse(name string) (Key, bool) {
	k, ok := lookup[strings.ToLower(strings.TrimSpace(name))]
	return k, ok
}

// IsSupported reports whether k is a canonical bindable key.
func IsSupported(k Key) bool {
	c, ok := lookup[strings.ToLower(string(k))]
	return ok && c == k
}

// Bindings holds the key for each action.
type Bindings struct {
	Normal  Key
	CloseUp Key
}

// DefaultBindings are F13 for normal framing and F14 for close-up.
func DefaultBindings() Bindings { return Bindings{Normal: "F13", CloseUp: "F14"} }

// Get returns the key bound to a.
func (b Bindings) Get(a Action) Key {
	if a == CloseUp {
		return b.CloseUp
	}
	return b.Normal
}

// With returns a copy of b with a bound to k.
func (b Bindings) With(a Action, k Key) Bindings {
	if a == CloseUp {
		b.CloseUp = k
	} else {
		b.Normal = k
	}
	return b
}

// Validate checks both keys are supported and distinct.
func (b Bindings) Validate() error {
	for _, a := range Actions {
		if !IsSupported(b.Get(a)) {
			return fmt.Errorf("%s: %q: %w", a, b.Get(a), ErrUnsupportedKey)
		}
	}
	if b.Normal == b.CloseUp {
		return fmt.Errorf("%s: %w", b.Normal, ErrDuplicate)
	}
	return nil
}

// CheckRebind validates binding a to k given the current bindings.
func CheckRebind(b Bindings, a Action, k Key) error {
	if k == Cancel || !IsSupported(k) {
		return fmt.Errorf("%s: %w", k, ErrUnsupportedKey)
	}
	if b.Get(a.Other()) == k {
		return fmt.Errorf("%s is already bound to %s: %w", k, a.Other(), ErrDuplicate)
	}
	return nil
}
