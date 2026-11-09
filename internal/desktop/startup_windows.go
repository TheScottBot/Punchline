//go:build windows

package desktop

// Start at login: the per-user Run key, the same value the install script
// writes. Task Manager's Startup tab records "disabled" in StartupApproved;
// its first byte is even when enabled and odd when disabled (observed
// behaviour, undocumented). Enabling here clears that override.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/thescottbot/punchline/internal/product"
)

const (
	runKeyPath      = `Software\Microsoft\Windows\CurrentVersion\Run`
	approvedKeyPath = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

func startupState(exe string) StartupState {
	st := StartupState{Available: true}
	if isTemporary(exe) {
		st.Available, st.Note = false, "unavailable from a temporary build"
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return st
	}
	defer k.Close()
	if _, _, err := k.GetStringValue(product.ShortName); err != nil {
		return st
	}
	st.Enabled = !approvedDisabled()
	return st
}

func approvedDisabled() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, approvedKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	b, _, err := k.GetBinaryValue(product.ShortName)
	return err == nil && len(b) > 0 && b[0]&1 == 1
}

func setStartup(exe string, on bool) error {
	if on && isTemporary(exe) {
		return errors.New("refusing to register a temporary build for startup")
	}
	if ak, err := registry.OpenKey(registry.CURRENT_USER, approvedKeyPath, registry.SET_VALUE); err == nil {
		ak.DeleteValue(product.ShortName)
		ak.Close()
	}
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if on {
		return k.SetStringValue(product.ShortName, `"`+exe+`"`)
	}
	if err := k.DeleteValue(product.ShortName); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

// isTemporary reports whether exe lives somewhere that will be deleted, such
// as a `go run` build folder. Startup must never point there.
func isTemporary(exe string) bool {
	l := strings.ToLower(filepath.Clean(exe))
	tmp := strings.ToLower(filepath.Clean(os.TempDir())) + `\`
	return strings.HasPrefix(l, tmp) || strings.Contains(l, `\go-build`)
}
