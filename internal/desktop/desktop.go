// Package desktop is the native desktop integration: tray / menu bar, global
// hotkeys, key capture for rebinding, start at login and single-instance
// enforcement. It is the only package (with internal/uvc) that touches the
// OS. It drives internal/app and holds no application logic of its own.
package desktop

import (
	"errors"
	"io"
	"log"
)

// ErrAlreadyRunning is returned by New when another instance holds the lock.
var ErrAlreadyRunning = errors.New("another instance is already running")

// Config configures the shell.
type Config struct {
	Logger      *log.Logger
	SettingsDir string // opened by "Open Settings Folder"
}

func (c *Config) defaults() {
	if c.Logger == nil {
		c.Logger = log.New(io.Discard, "", 0)
	}
}
