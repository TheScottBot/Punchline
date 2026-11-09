package uvc

import "testing"

func TestParseVideoControl(t *testing.T) {
	cfg := []byte{
		9, 0x02, 0, 0, 2, 1, 0, 0x80, 250, // configuration (wTotalLength not checked)
		8, 0x0b, 0, 2, 0x0e, 0x03, 0, 0, // interface association
		9, usbDescInterface, 0, 0, 1, 0x0e, 0x01, 0, 0, // VC interface 0
		13, csInterface, 0x01, 0x00, 0x01, 0, 0, 0, 0, 0, 0, 1, 1, // VC header
		// camera terminal ID 1, bControlSize 3, bmControls 0x0a 0x02 0x00 (zoom absolute = bit 9)
		18, csInterface, vcInputTerminal, 1, 0x01, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0x0a, 0x02, 0x00,
		9, usbDescInterface, 1, 0, 0, 0x0e, 0x02, 0, 0, // VS interface 1
		9, usbDescInterface, 2, 0, 0, 0x0e, 0x01, 0, 0, // second VC interface, no zoom
		18, csInterface, vcInputTerminal, 5, 0x01, 0x02, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0xff, 0x00, 0x00,
		9, usbDescInterface, 3, 0, 0, 0x01, 0x01, 0, 0, // audio control: ignored
		8, csInterface, vcInputTerminal, 9, 0x01, 0x02, 0, 0, // not in a VC interface
	}
	got := parseVideoControl(cfg)
	want := []vcFunction{
		{Interface: 0, HasCamera: true, TerminalID: 1, Zoom: true},
		{Interface: 2, HasCamera: true, TerminalID: 5, Zoom: false},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] got %+v want %+v", i, got[i], want[i])
		}
	}
}

func TestParseVideoControlTruncated(t *testing.T) {
	cfg := []byte{9, usbDescInterface, 0, 0, 1, 0x0e, 0x01, 0, 0, 18, csInterface, vcInputTerminal, 1}
	got := parseVideoControl(cfg)
	if len(got) != 1 || got[0].HasCamera {
		t.Fatalf("got %+v", got)
	}
}
