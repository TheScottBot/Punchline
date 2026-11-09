package settings

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMissingFileGivesDefaults(t *testing.T) {
	s, p, err := Load(filepath.Join(t.TempDir(), FileName))
	if err != nil || len(p) != 0 {
		t.Fatal(err, p)
	}
	if s.NormalKey != "F13" || s.CloseUpKey != "F14" || !s.HotkeysEnabled || s.NormalZoom != 1 || s.CloseUpZoom != 0 {
		t.Fatalf("%+v", s)
	}
}

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", FileName)
	in := Defaults()
	in.NormalKey, in.CloseUpKey = "F20", "Q"
	in.HotkeysEnabled = false
	in.PreferredCamera = "streamcam:046d:0893:X"
	in.CloseUpZoom = 1.5
	in.CameraZoom["streamcam:046d:0893:X"] = ZoomOverride{CloseUp: 2}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, p, err := Load(path)
	if err != nil || len(p) != 0 {
		t.Fatal(err, p)
	}
	if out.NormalKey != "F20" || out.CloseUpKey != "Q" || out.HotkeysEnabled || out.PreferredCamera != in.PreferredCamera ||
		out.CloseUpZoom != 1.5 || out.CameraZoom["streamcam:046d:0893:X"].CloseUp != 2 {
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
	path := write(t, `{"version":1,"normalKey":"Shift","closeUpKey":"f15","normalZoom":99,"closeUpZoom":"x","bogus":1,"hotkeysEnabled":false}`)
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
