package settings

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/thescottbot/punchline/internal/hotkey"
)

func TestMissingFileGivesDefaults(t *testing.T) {
	s, p, err := Load(filepath.Join(t.TempDir(), FileName))
	if err != nil || len(p) != 0 {
		t.Fatal(err, p)
	}
	if s.NormalKey != "F13" || s.HalfwayKey != "F15" || s.CloseUpKey != "F14" || !s.HotkeysEnabled || s.NormalZoom != 1 || s.HalfwayZoom != 0 || s.CloseUpZoom != 0 {
		t.Fatalf("%+v", s)
	}
}

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	in := Defaults()
	in.NormalKey, in.HalfwayKey, in.CloseUpKey = "F20", "W", "Q"
	in.HotkeysEnabled = false
	in.PreferredCamera = "streamcam:046d:0893:X"
	in.CloseUpZoom = 1.5
	in.HalfwayZoom = 1.25
	in.CameraZoom["streamcam:046d:0893:X"] = ZoomOverride{CloseUp: 2, Halfway: 1.5}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, p, err := Load(path)
	if err != nil || len(p) != 0 {
		t.Fatal(err, p)
	}
	if out.NormalKey != "F20" || out.HalfwayKey != "W" || out.CloseUpKey != "Q" || out.HotkeysEnabled || out.PreferredCamera != in.PreferredCamera ||
		out.CloseUpZoom != 1.5 || out.HalfwayZoom != 1.25 || out.CameraZoom["streamcam:046d:0893:X"] != (ZoomOverride{CloseUp: 2, Halfway: 1.5}) {
		t.Fatalf("%+v", out)
	}
}

func write(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), FileName)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestInvalidFieldsFallBack(t *testing.T) {
	path := write(t, `{"version":1,"normalKey":"Shift","halfwayKey":"F16","closeUpKey":"f15","normalZoom":99,"closeUpZoom":"x","bogus":1,"hotkeysEnabled":false}`)
	s, p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.NormalKey != "F13" || s.CloseUpKey != "F15" || s.NormalZoom != 1 || s.CloseUpZoom != 0 || s.HotkeysEnabled {
		t.Fatalf("%+v", s)
	}
	if len(p) != 4 {
		t.Fatalf("problems %q", p)
	}
	for _, x := range p {
		if strings.Contains(x, "Shift") {
			t.Fatal("problem leaks file content")
		}
	}
}

func TestDuplicateKeysReset(t *testing.T) {
	s, p, _ := Load(write(t, `{"version":1,"normalKey":"F14","closeUpKey":"F14"}`))
	if s.NormalKey != "F13" || s.CloseUpKey != "F14" || len(p) != 1 {
		t.Fatalf("%+v %q", s, p)
	}
}

// A settings file from before the halfway level, whose close-up key is F15,
// keeps its bindings; halfway moves to the next free F-key.
func TestHalfwayKeyMovedOffClash(t *testing.T) {
	s, p, _ := Load(write(t, `{"version":1,"normalKey":"F13","closeUpKey":"F15"}`))
	if s.NormalKey != "F13" || s.CloseUpKey != "F15" || s.HalfwayKey != "F16" || len(p) != 1 {
		t.Fatalf("%+v %q", s, p)
	}
	if err := s.Bindings().Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestUnreadableFileSetAside(t *testing.T) {
	path := write(t, `not json`)
	s, p, err := Load(path)
	if err != nil || s.NormalKey != "F13" || len(p) == 0 {
		t.Fatal(err, s, p)
	}
	if _, err := os.Stat(path + ".invalid"); err != nil {
		t.Fatal("not set aside")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("original still present")
	}
}

func TestWrongVersionSetAside(t *testing.T) {
	path := write(t, `{"version":2}`)
	if _, p, _ := Load(path); len(p) == 0 {
		t.Fatal("no problem reported")
	}
}

func TestHalfwayZoomValidation(t *testing.T) {
	s, p, _ := Load(write(t, `{"version":1,"halfwayZoom":99}`))
	if s.HalfwayZoom != 0 || len(p) != 1 || !strings.Contains(p[0], "halfwayZoom") {
		t.Fatalf("%v %q", s.HalfwayZoom, p)
	}
	s, p, _ = Load(write(t, `{"version":1,"halfwayZoom":2.5}`))
	if s.HalfwayZoom != 2.5 || len(p) != 0 {
		t.Fatalf("%v %q", s.HalfwayZoom, p)
	}
}

func TestCameraZoomBadEntryDropped(t *testing.T) {
	s, p, _ := Load(write(t, `{"version":1,"cameraZoom":{"good":{"halfway":2},"bad":{"halfway":0.5},"worse":{"closeUp":40}}}`))
	if _, ok := s.CameraZoom["good"]; !ok || len(s.CameraZoom) != 1 || len(p) != 2 {
		t.Fatalf("%+v %q", s.CameraZoom, p)
	}
	s, p, _ = Load(write(t, `{"version":1,"cameraZoom":[1,2]}`))
	if len(s.CameraZoom) != 0 || len(p) != 1 {
		t.Fatalf("%+v %q", s.CameraZoom, p)
	}
}

func TestFreeKeyFallsBackWhenEverythingIsTaken(t *testing.T) {
	var all []string
	for n := 13; n <= 24; n++ {
		all = append(all, "F"+strconv.Itoa(n))
	}
	if got := freeKey(all...); got != "F15" {
		t.Fatalf("got %s", got)
	}
	if got := freeKey("F15", "F16"); got != "F17" {
		t.Fatalf("got %s", got)
	}
}

func TestCloneIsIndependent(t *testing.T) {
	a := Defaults()
	a.CameraZoom["x"] = ZoomOverride{CloseUp: 2}
	b := a.Clone()
	b.CameraZoom["x"] = ZoomOverride{CloseUp: 3}
	b.CameraZoom["y"] = ZoomOverride{}
	if a.CameraZoom["x"].CloseUp != 2 || len(a.CameraZoom) != 1 {
		t.Fatal("clone shares the override map")
	}
}

func TestBindingsRoundTrip(t *testing.T) {
	s := Defaults().WithBindings(hotkey.Bindings{Normal: "A", Halfway: "B", CloseUp: "C"})
	if b := s.Bindings(); b.Normal != "A" || b.Halfway != "B" || b.CloseUp != "C" {
		t.Fatalf("%+v", b)
	}
}

func TestSaveFailureLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.Mkdir(path, 0o700); err != nil { // a directory where the file should go
		t.Fatal(err)
	}
	if err := Save(path, Defaults()); err == nil {
		t.Fatal("save over a directory succeeded")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("leftovers: %v", entries)
	}
}

func TestFileStoreAndDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	if err := FileStore(path).Save(Defaults()); err != nil {
		t.Fatal(err)
	}
	if _, p, err := Load(path); err != nil || len(p) != 0 {
		t.Fatal(err, p)
	}
	d, err := Dir()
	if err != nil || filepath.Base(d) != "Punchline" {
		t.Fatalf("%q %v", d, err)
	}
}

func TestMIDIBindingsRoundTrip(t *testing.T) {
	s, p, _ := Load(write(t, `{"version":1,"closeUpKey":"midi cc 1/32","halfwayKey":"MIDI Note 10/36"}`))
	if s.CloseUpKey != "MIDI CC 1/32" || s.HalfwayKey != "MIDI Note 10/36" || s.NormalKey != "F13" || len(p) != 0 {
		t.Fatalf("%+v %q", s, p)
	}
	_, p, _ = Load(write(t, `{"version":1,"closeUpKey":"midi cc 99/1"}`))
	if len(p) != 1 {
		t.Fatalf("bad MIDI accepted: %q", p)
	}
}
