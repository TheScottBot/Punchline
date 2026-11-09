//go:build !windows

package desktop

import (
	"errors"

	"github.com/thescottbot/punchline/internal/app"
	"github.com/thescottbot/punchline/internal/hotkey"
)

// ErrUnsupportedPlatform: the macOS menu bar shell is not yet implemented.
var ErrUnsupportedPlatform = errors.New("desktop: not yet implemented on this platform")

// Shell is a placeholder until the macOS implementation lands.
type Shell struct{}

func New(cfg Config) (*Shell, error)                        { return nil, ErrUnsupportedPlatform }
func (s *Shell) Registrar() app.Registrar                   { return noReg{} }
func (s *Shell) Changed()                                   {}
func (s *Shell) Run(core *app.Core, notices []string) error { return ErrUnsupportedPlatform }
func RequestQuit() bool                                     { return false }

type noReg struct{}

func (noReg) Register(hotkey.Action, hotkey.Key) error { return ErrUnsupportedPlatform }
func (noReg) Unregister(hotkey.Action)                 {}
