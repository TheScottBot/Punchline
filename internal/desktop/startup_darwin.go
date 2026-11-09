//go:build darwin

package desktop

// Start at login: a per-user LaunchAgent, the same file the install script
// writes. Deleting it is enough to stop the next login launch; the running
// instance is never booted out from here (that would kill this process).

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/thescottbot/punchline/internal/product"
)

func launchAgentPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", product.BundleID+".plist"), nil
}

func startupState(exe string) StartupState {
	st := StartupState{Available: true}
	if isTemporary(exe) {
		st.Available, st.Note = false, "unavailable from a temporary build"
	}
	if p, err := launchAgentPath(); err == nil {
		if _, err := os.Stat(p); err == nil {
			st.Enabled = true
		}
	}
	return st
}

func setStartup(exe string, on bool) error {
	p, err := launchAgentPath()
	if err != nil {
		return err
	}
	if !on {
		if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return nil
	}
	if isTemporary(exe) {
		return errors.New("refusing to register a temporary build for startup")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, launchAgentPlist(exe), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// launchAgentPlist must stay in step with scripts/macos/punchline.sh.
func launchAgentPlist(exe string) []byte {
	var esc bytes.Buffer
	xml.EscapeText(&esc, []byte(exe))
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + product.BundleID + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + esc.String() + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>LimitLoadToSessionType</key>
	<string>Aqua</string>
</dict>
</plist>
`)
}

// isTemporary reports whether exe lives somewhere that will be deleted, such
// as a `go run` build folder. Startup must never point there.
func isTemporary(exe string) bool {
	clean := func(p string) string {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			p = r
		}
		return filepath.Clean(p)
	}
	e := clean(exe)
	for _, dir := range []string{os.TempDir(), "/tmp", "/private/tmp", "/var/tmp"} {
		if strings.HasPrefix(e, clean(dir)+"/") {
			return true
		}
	}
	return strings.Contains(e, "/go-build")
}
