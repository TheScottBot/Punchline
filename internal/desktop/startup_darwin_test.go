//go:build darwin

package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsTemporary(t *testing.T) {
	if !isTemporary(filepath.Join(os.TempDir(), "go-build123", "b001", "exe", "punchline")) {
		t.Error("go run build not temporary")
	}
	if isTemporary("/Users/x/Applications/Disciplinary Close-Up.app/Contents/MacOS/punchline") {
		t.Error("installed app reported temporary")
	}
}

func TestLaunchAgentPlistEscapes(t *testing.T) {
	p := string(launchAgentPlist("/Users/a&b/App <1>.app/Contents/MacOS/punchline"))
	if !strings.Contains(p, "<string>/Users/a&amp;b/App &lt;1&gt;.app/Contents/MacOS/punchline</string>") {
		t.Fatal(p)
	}
	if !strings.Contains(p, "<string>com.thescottbot.punchline</string>") {
		t.Fatal("label missing")
	}
}
