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
	if err := (Bindings{"F13", "F13"}).Validate(); !errors.Is(err, ErrDuplicate) {
		t.Errorf("duplicate: %v", err)
	}
	if err := (Bindings{"F13", "Nope"}).Validate(); !errors.Is(err, ErrUnsupportedKey) {
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
}
