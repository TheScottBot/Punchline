package midi

import "testing"

func TestDecode(t *testing.T) {
	cases := []struct {
		in    [3]byte
		want  Message
		press bool
		str   string
	}{
		{[3]byte{0xb0, 3, 127}, Message{Kind: ControlChange, Channel: 1, Number: 3, Value: 127}, true, "CC ch 1 #3 value 127"},
		{[3]byte{0xb0, 3, 0}, Message{Kind: ControlChange, Channel: 1, Number: 3}, false, "CC ch 1 #3 value 0"},
		{[3]byte{0x9f, 36, 100}, Message{Kind: NoteOn, Channel: 16, Number: 36, Value: 100}, true, "note on ch 16 note 36 value 100"},
		{[3]byte{0x90, 36, 0}, Message{Kind: NoteOn, Channel: 1, Number: 36}, false, ""}, // velocity 0 = release
		{[3]byte{0x81, 36, 64}, Message{Kind: NoteOff, Channel: 2, Number: 36, Value: 64}, false, ""},
		{[3]byte{0xa0, 1, 2}, Message{Kind: PolyPressure, Channel: 1, Number: 1, Value: 2}, false, ""},
		{[3]byte{0xc5, 9, 99}, Message{Kind: ProgramChange, Channel: 6, Number: 9}, false, "program ch 6 #9"},
		{[3]byte{0xd0, 77, 0}, Message{Kind: ChannelPressure, Channel: 1, Value: 77}, false, ""},
		{[3]byte{0xe0, 0, 64}, Message{Kind: PitchBend, Channel: 1, Value: 64}, false, ""},
		{[3]byte{0xf8, 0, 0}, Message{}, false, "system F8"},
		{[3]byte{0x40, 0, 0}, Message{}, false, ""}, // data byte as status: not decoded
	}
	for _, c := range cases {
		got := Decode(c.in[0], c.in[1], c.in[2])
		want := c.want
		want.Raw = [3]byte{c.in[0], c.in[1] & 0x7f, c.in[2] & 0x7f}
		if got != want {
			t.Errorf("% X: got %+v want %+v", c.in, got, want)
		}
		if got.IsPress() != c.press {
			t.Errorf("% X: IsPress %v", c.in, got.IsPress())
		}
		if c.str != "" && got.String() != c.str {
			t.Errorf("% X: String %q want %q", c.in, got.String(), c.str)
		}
	}
	if !Decode(0xfe, 0, 0).IsRealtime() || Decode(0xb0, 1, 1).IsRealtime() {
		t.Error("IsRealtime")
	}
	if Decode(0xb0, 0xff, 0xff) != (Message{Kind: ControlChange, Channel: 1, Number: 127, Value: 127, Raw: [3]byte{0xb0, 0x7f, 0x7f}}) {
		t.Error("data bytes not masked")
	}
}
