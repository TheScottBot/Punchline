//go:build windows

package desktop

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"

	"github.com/thescottbot/punchline/internal/hotkey"
)

// Virtual-key codes for each supported key name. Punctuation names follow
// the US layout position of the OEM key.
var vkByKey = func() map[hotkey.Key]uint32 {
	m := map[hotkey.Key]uint32{
		"NumMultiply": 0x6A, "NumAdd": 0x6B, "NumSubtract": 0x6D, "NumDecimal": 0x6E, "NumDivide": 0x6F,
		"Space": 0x20, "Tab": 0x09, "Enter": 0x0D, "Backspace": 0x08,
		"Insert": 0x2D, "Delete": 0x2E, "Home": 0x24, "End": 0x23, "PageUp": 0x21, "PageDown": 0x22,
		"Left": 0x25, "Up": 0x26, "Right": 0x27, "Down": 0x28,
		"Pause": 0x13, "ScrollLock": 0x91, "PrintScreen": 0x2C, "Menu": 0x5D,
		"Minus": 0xBD, "Equals": 0xBB, "LeftBracket": 0xDB, "RightBracket": 0xDD, "Backslash": 0xDC,
		"Semicolon": 0xBA, "Quote": 0xDE, "Backquote": 0xC0, "Comma": 0xBC, "Period": 0xBE, "Slash": 0xBF,
		"VolumeMute": 0xAD, "VolumeDown": 0xAE, "VolumeUp": 0xAF,
		"MediaNext": 0xB0, "MediaPrevious": 0xB1, "MediaStop": 0xB2, "MediaPlayPause": 0xB3,
	}
	for i := 0; i < 24; i++ {
		m[hotkey.Key(fmt.Sprintf("F%d", i+1))] = 0x70 + uint32(i)
	}
	for i := 0; i < 26; i++ {
		m[hotkey.Key(string(rune('A'+i)))] = 0x41 + uint32(i)
	}
	for i := 0; i < 10; i++ {
		m[hotkey.Key(string(rune('0'+i)))] = 0x30 + uint32(i)
		m[hotkey.Key(fmt.Sprintf("Num%d", i))] = 0x60 + uint32(i)
	}
	return m
}()

var keyByVK = func() map[uint32]hotkey.Key {
	m := make(map[uint32]hotkey.Key, len(vkByKey))
	for k, v := range vkByKey {
		m[v] = k
	}
	return m
}()

func isModifierVK(vk uint32) bool {
	switch vk {
	case 0x10, 0x11, 0x12, 0xA0, 0xA1, 0xA2, 0xA3, 0xA4, 0xA5, 0x5B, 0x5C:
		return true
	}
	return false
}

// winRegistrar implements app.Registrar with RegisterHotKey. Each
// registration gets a fresh ID so a replacement can be registered before the
// old one is released; if registration fails the old one is untouched.
type winRegistrar struct {
	hwnd    windows.HWND
	ids     map[hotkey.Action]uintptr
	actions map[uintptr]hotkey.Action
	next    uintptr
}

func newRegistrar(h windows.HWND) *winRegistrar {
	return &winRegistrar{hwnd: h, ids: map[hotkey.Action]uintptr{}, actions: map[uintptr]hotkey.Action{}, next: 1}
}

func (r *winRegistrar) Register(a hotkey.Action, k hotkey.Key) error {
	vk, ok := vkByKey[k]
	if !ok {
		return fmt.Errorf("%s: %w", k, hotkey.ErrUnsupportedKey)
	}
	id := r.next
	r.next++
	if r.next > 0xBFFF {
		r.next = 1
	}
	ok2, _, err := procRegisterHotKey.Call(uintptr(r.hwnd), id, modNoRepeat, uintptr(vk))
	if ok2 == 0 {
		var en windows.Errno
		if errors.As(err, &en) && en == errorHotkeyAlreadyRegistered {
			return fmt.Errorf("%s is in use by another application or reserved by Windows: %w", k, hotkey.ErrUnavailable)
		}
		return fmt.Errorf("%s: %v: %w", k, err, hotkey.ErrUnavailable)
	}
	if old, had := r.ids[a]; had {
		procUnregisterHotKey.Call(uintptr(r.hwnd), old)
		delete(r.actions, old)
	}
	r.ids[a] = id
	r.actions[id] = a
	return nil
}

func (r *winRegistrar) Unregister(a hotkey.Action) {
	if id, had := r.ids[a]; had {
		procUnregisterHotKey.Call(uintptr(r.hwnd), id)
		delete(r.actions, id)
		delete(r.ids, a)
	}
}

func (r *winRegistrar) action(id uintptr) (hotkey.Action, bool) {
	a, ok := r.actions[id]
	return a, ok
}
