package uvc

import "testing"

func TestParseUSBIDs(t *testing.T) {
	v, p, ok := parseUSBIDs(`\\?\usb#vid_046d&pid_0893&mi_00#7&2a1b3c&0&0000#{65e8773d-8f56-11d0-a3b9-00a0c9223196}\global`)
	if !ok || v != 0x046d || p != 0x0893 {
		t.Fatalf("got %04x %04x %v", v, p, ok)
	}
	if _, _, ok := parseUSBIDs(`\\?\root#image#0000`); ok {
		t.Fatal("non-USB path parsed")
	}
}
