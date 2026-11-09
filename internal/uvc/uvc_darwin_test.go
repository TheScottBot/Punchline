//go:build darwin

package uvc

import (
	"errors"
	"strings"
	"testing"
)

func TestParsePath(t *testing.T) {
	e, i, err := parsePath("iokit:100006a05:0")
	if err != nil || e != 0x100006a05 || i != 0 {
		t.Fatal(e, i, err)
	}
	if _, i, err := parsePath("iokit:1f:-"); err != nil || i != -1 {
		t.Fatal(i, err)
	}
	for _, bad := range []string{"", "iokit:zz:0", "iokit:1:x", "dshow:1:0", "iokit:1"} {
		if _, _, err := parsePath(bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestIOReturnErrorText(t *testing.T) {
	if s := (&IOReturnError{Op: "SET_CUR zoom", Code: 0xe00002c0}).Error(); !strings.Contains(s, "kIOReturnNoDevice") {
		t.Error(s)
	}
	if s := (&IOReturnError{Op: "x", Code: 0xe0001234}).Error(); !strings.Contains(s, "0xe0001234") {
		t.Error(s)
	}
	if ioErr("x", 0) != nil {
		t.Error("success reported as error")
	}
}

func TestOpenBadPath(t *testing.T) {
	if _, err := System().Open("nonsense"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := System().Open("iokit:ffffffffffff:0"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing device: %v", err)
	}
}
