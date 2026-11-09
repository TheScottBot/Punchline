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

// The install script writes the same LaunchAgent as the menu toggle.
func TestScriptPlistMatchesApp(t *testing.T) {
	b, err := os.ReadFile("../../scripts/macos/punchline.sh")
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	const open = "cat > \"$PLIST.tmp\" <<EOF\n"
	i := strings.Index(s, open)
	if i < 0 {
		t.Fatal("plist heredoc not found in script")
	}
	body := s[i+len(open):]
	body = body[:strings.Index(body, "\nEOF\n")+1]
	exe := "/Users/a&b/Applications/Disciplinary Close-Up.app/Contents/MacOS/punchline"
	body = strings.NewReplacer(
		"$LABEL", "com.thescottbot.punchline",
		`$(xml_escape "$EXE")`, "/Users/a&amp;b/Applications/Disciplinary Close-Up.app/Contents/MacOS/punchline",
	).Replace(body)
	if want := string(launchAgentPlist(exe)); body != want {
		t.Fatalf("script plist differs from app plist:\n--- script\n%s\n--- app\n%s", body, want)
	}
}
