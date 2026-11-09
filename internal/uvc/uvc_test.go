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

func TestPortless(t *testing.T) {
	nodes := []Node{
		{VendorID: 0x2e1a, ProductID: 0x4c06, StableID: "2e1a:4c06@loc20440000"},
		{VendorID: 0x291a, ProductID: 0x3369, StableID: "291a:3369:ACNV9P1F20606968"},
		{VendorID: 0x046d, ProductID: 0x0893, StableID: "046d:0893@loc1"},
		{VendorID: 0x046d, ProductID: 0x0893, StableID: "046d:0893@loc2"},
		{VendorID: 0x1234, ProductID: 0x0001, StableID: "1234:0001@loc3#0"},
		{VendorID: 0x1234, ProductID: 0x0001, StableID: "1234:0001@loc3#2"},
		{StableID: "path:root\\image\\0000"},
	}
	portless(nodes)
	want := []string{
		"2e1a:4c06",
		"291a:3369:ACNV9P1F20606968",
		"046d:0893@loc1",
		"046d:0893@loc2",
		"1234:0001#0",
		"1234:0001#2",
		"path:root\\image\\0000",
	}
	for i, w := range want {
		if nodes[i].StableID != w {
			t.Errorf("[%d] got %q want %q", i, nodes[i].StableID, w)
		}
	}
}
