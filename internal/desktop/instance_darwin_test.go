//go:build darwin

package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

func TestSingleInstanceLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Punchline") // created on demand
	f, held, err := tryLock(dir)
	if err != nil || held || f == nil {
		t.Fatal(f, held, err)
	}
	b, _ := os.ReadFile(lockPath(dir))
	if strings.TrimSpace(string(b)) != strconv.Itoa(os.Getpid()) {
		t.Fatalf("lock file holds %q", b)
	}
	// flock is per open file, so a second open in this process conflicts
	// just as a second launch would.
	if g, held, err := tryLock(dir); err != nil || !held || g != nil {
		t.Fatal("second lock not refused", g, held, err)
	}
	f.Close()
	g, held, err := tryLock(dir)
	if err != nil || held {
		t.Fatal("lock not released on close", held, err)
	}
	g.Close()
}

func TestSignalRunning(t *testing.T) {
	dir := t.TempDir()
	if signalRunning(dir, 0) {
		t.Fatal("signalled with no lock file")
	}
	os.WriteFile(lockPath(dir), []byte("garbage\n"), 0o600)
	if signalRunning(dir, 0) {
		t.Fatal("signalled a garbage PID")
	}
	os.WriteFile(lockPath(dir), []byte(strconv.Itoa(os.Getpid())+"\n"), 0o600)
	if signalRunning(dir, 0) {
		t.Fatal("signalled itself")
	}
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	os.WriteFile(lockPath(dir), []byte(strconv.Itoa(cmd.Process.Pid)+"\n"), 0o600)
	if !signalRunning(dir, syscall.SIGTERM) {
		t.Fatal("did not signal the holder")
	}
	if err := cmd.Wait(); err == nil || !strings.Contains(err.Error(), "terminated") {
		t.Fatalf("holder not terminated: %v", err)
	}
}

func TestStartupToggleWritesAndRemovesLaunchAgent(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	exe := "/Users/someone/Applications/Disciplinary Close-Up.app/Contents/MacOS/punchline"
	if st := startupState(exe); st.Enabled || !st.Available {
		t.Fatalf("initial %+v", st)
	}
	if err := setStartup(exe, true); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, "Library", "LaunchAgents", "com.thescottbot.punchline.plist")
	b, err := os.ReadFile(p)
	if err != nil || string(b) != string(launchAgentPlist(exe)) {
		t.Fatalf("plist %v:\n%s", err, b)
	}
	if !startupState(exe).Enabled {
		t.Fatal("not reported enabled")
	}
	if err := setStartup(exe, true); err != nil { // idempotent
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // disabling twice is fine
		if err := setStartup(exe, false); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) || startupState(exe).Enabled {
		t.Fatal("plist not removed")
	}
	entries, _ := os.ReadDir(filepath.Dir(p))
	if len(entries) != 0 {
		t.Fatalf("leftovers %v", entries)
	}
}

func TestStartupRefusesTemporaryBuild(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	tmp := filepath.Join(os.TempDir(), "go-build1", "exe", "punchline")
	if st := startupState(tmp); st.Available || st.Note == "" {
		t.Fatalf("%+v", st)
	}
	if err := setStartup(tmp, true); err == nil {
		t.Fatal("registered a temporary build")
	}
	for _, p := range []string{"/tmp/x/punchline", "/private/tmp/x/punchline"} {
		if !isTemporary(p) {
			t.Errorf("%s not temporary", p)
		}
	}
}

func TestIconPixelConversion(t *testing.T) {
	// One opaque blue-ish pixel and one half-transparent white pixel, BGRA.
	got := toRGBAPremultiplied([]byte{200, 100, 50, 255, 255, 255, 255, 128})
	want := []byte{50, 100, 200, 255, 128, 128, 128, 128}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
