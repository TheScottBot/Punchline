//go:build darwin

package desktop

import "github.com/thescottbot/punchline/internal/hotkey"

// macOS virtual key codes (Carbon kVK_*, HIToolbox/Events.h). These are
// layout-independent physical positions, so "A" means the key in the ANSI A
// position.
var codeByKey = map[hotkey.Key]uint16{
	"F1": 0x7A, "F2": 0x78, "F3": 0x63, "F4": 0x76, "F5": 0x60, "F6": 0x61,
	"F7": 0x62, "F8": 0x64, "F9": 0x65, "F10": 0x6D, "F11": 0x67, "F12": 0x6F,
	"F13": 0x69, "F14": 0x6B, "F15": 0x71, "F16": 0x6A, "F17": 0x40, "F18": 0x4F,
	"F19": 0x50, "F20": 0x5A,

	"A": 0x00, "S": 0x01, "D": 0x02, "F": 0x03, "H": 0x04, "G": 0x05, "Z": 0x06,
	"X": 0x07, "C": 0x08, "V": 0x09, "B": 0x0B, "Q": 0x0C, "W": 0x0D, "E": 0x0E,
	"R": 0x0F, "Y": 0x10, "T": 0x11, "O": 0x1F, "U": 0x20, "I": 0x22, "P": 0x23,
	"L": 0x25, "J": 0x26, "K": 0x28, "N": 0x2D, "M": 0x2E,

	"1": 0x12, "2": 0x13, "3": 0x14, "4": 0x15, "6": 0x16, "5": 0x17, "9": 0x19,
	"7": 0x1A, "8": 0x1C, "0": 0x1D,

	"Num0": 0x52, "Num1": 0x53, "Num2": 0x54, "Num3": 0x55, "Num4": 0x56,
	"Num5": 0x57, "Num6": 0x58, "Num7": 0x59, "Num8": 0x5B, "Num9": 0x5C,
	"NumMultiply": 0x43, "NumAdd": 0x45, "NumSubtract": 0x4E, "NumDecimal": 0x41,
	"NumDivide": 0x4B,

	"Space": 0x31, "Tab": 0x30, "Enter": 0x24, "Backspace": 0x33,
	"Insert": 0x72, // kVK_Help: the Insert position on PC keyboards
	"Delete": 0x75, "Home": 0x73, "End": 0x77, "PageUp": 0x74, "PageDown": 0x79,
	"Up": 0x7E, "Down": 0x7D, "Left": 0x7B, "Right": 0x7C,
	"Menu": 0x6E, // kVK_ContextualMenu

	"Minus": 0x1B, "Equals": 0x18, "LeftBracket": 0x21, "RightBracket": 0x1E,
	"Backslash": 0x2A, "Semicolon": 0x29, "Quote": 0x27, "Backquote": 0x32,
	"Comma": 0x2B, "Period": 0x2F, "Slash": 0x2C,
}

// unsupportedOnMac lists bindable keys macOS cannot deliver as a hotkey:
// there are no virtual key codes beyond F20; PC Print Screen / Scroll Lock /
// Pause arrive as F13 / F14 / F15; volume and media keys are system-defined
// events, not key codes.
var unsupportedOnMac = map[hotkey.Key]bool{
	"F21": true, "F22": true, "F23": true, "F24": true,
	"PrintScreen": true, "ScrollLock": true, "Pause": true,
	"VolumeMute": true, "VolumeDown": true, "VolumeUp": true,
	"MediaNext": true, "MediaPrevious": true, "MediaStop": true, "MediaPlayPause": true,
}

const (
	kvkEscape      = 0x35
	kvkKeypadEnter = 0x4C
)

var keyByCode = func() map[uint16]hotkey.Key {
	m := make(map[uint16]hotkey.Key, len(codeByKey)+1)
	for k, c := range codeByKey {
		m[c] = k
	}
	m[kvkKeypadEnter] = "Enter" // both Enter keys bind as Enter
	return m
}()
