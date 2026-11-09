//go:build !windows

package uvc

// System returns the platform backend. The macOS IOKit backend is not yet
// implemented; see docs/PLAN.md.
func System() Backend { return unsupported{} }

type unsupported struct{}

func (unsupported) Enumerate() ([]Node, error)   { return nil, ErrUnsupportedPlatform }
func (unsupported) Open(string) (Control, error) { return nil, ErrUnsupportedPlatform }
