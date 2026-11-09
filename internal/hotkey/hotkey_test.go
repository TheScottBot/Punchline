package hotkey

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	for in, want := range map[string]Key{"f13": "F13", " F24 ": "F24", "a": "A", "num5": "Num5", "pageup": "PageUp"} {
		got, ok := Parse(in)
		if !ok || got != want {
			t.Errorf("Parse(%q) = %q, %v; want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"", "F25", "Escape", "Shift", "Ctrl+F13"} {
		if _, ok := Parse(in); ok {
			t.Errorf("Parse(%q) accepted", in)
		}
	}
}

func TestExtendedFunctionKeysSupported(t *testing.T) {
	for _, k := range []Key{"F13", "F14", "F15", "F16", "F17", "F18", "F19", "F20", "F21", "F22", "F23", "F24"} {
		if !IsSupported(k) {
			t.Errorf("%s unsupported", k)
		}
	}
	if IsSupported("f13") {
		t.Error("non-canonical name accepted by IsSupported")
	}
}

func TestBindingsValidate(t *testing.T) {
	if err := DefaultBindings().Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (Bindings{Normal: "F13", Halfway: "F15", CloseUp: "F13"}).Validate(); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate: %v", err)
	}
	if err := (Bindings{Normal: "F13", Halfway: "F14", CloseUp: "F14"}).Validate(); !errors.Is(err, ErrDuplicate) {
		t.Errorf("halfway duplicate: %v", err)
	}
	if err := (Bindings{Normal: "F13", Halfway: "F15", CloseUp: "Nope"}).Validate(); !errors.Is(err, ErrUnsupportedKey) {
		t.Errorf("unsupported: %v", err)
	}
}

func TestCheckRebind(t *testing.T) {
	b := DefaultBindings()
	if err := CheckRebind(b, CloseUp, "F13"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate: %v", err)
	}
	if err := CheckRebind(b, CloseUp, Cancel); !errors.Is(err, ErrUnsupportedKey) {
		t.Errorf("escape: %v", err)
	}
	if err := CheckRebind(b, CloseUp, "F14"); err != nil {
		t.Errorf("same key: %v", err)
	}
	if err := CheckRebind(b, CloseUp, "Z"); err != nil {
		t.Errorf("letter: %v", err)
	}
	if err := CheckRebind(b, Normal, "F15"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("halfway key: %v", err)
	}
	if err := CheckRebind(b, Halfway, "F14"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("halfway onto close-up: %v", err)
	}
}

func TestMIDIBindings(t *testing.T) {
	for in, want := range map[string]Key{
		"midi cc 1/32": "MIDI CC 1/32", " MIDI CC 16 127 ": "MIDI CC 16/127", "midi note 10/36": "MIDI Note 10/36",
	} {
		got, ok := Parse(in)
		if !ok || got != want || !IsSupported(got) || !got.IsMIDI() {
			t.Errorf("Parse(%q) = %q, %v", in, got, ok)
		}
	}
	for _, in := range []string{"midi cc 0/1", "midi cc 17/1", "midi cc 1/128", "midi pc 1/2", "midi cc 1", "midi cc a/b"} {
		if _, ok := Parse(in); ok {
			t.Errorf("Parse(%q) accepted", in)
		}
	}
	if IsSupported("midi cc 1/32") {
		t.Error("non-canonical MIDI accepted by IsSupported")
	}
	if tr, ok := Key("MIDI CC 1/32").MIDI(); !ok || tr != (MIDITrigger{Channel: 1, Number: 32}) {
		t.Errorf("%+v %v", tr, ok)
	}
	if Key("F13").IsMIDI() {
		t.Error("F13 is not MIDI")
	}
	b := DefaultBindings().With(CloseUp, "MIDI CC 1/32")
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := CheckRebind(b, Normal, "MIDI CC 1/32"); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate MIDI: %v", err)
	}
	if err := CheckRebind(b, Normal, "MIDI CC 1/33"); err != nil {
		t.Errorf("free MIDI: %v", err)
	}
}
