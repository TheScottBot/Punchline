//go:build !windows && !darwin

package uvc

// System returns the platform backend. Only Windows and macOS are supported.
func System() Backend { return unsupported{} }

type unsupported struct{}

func (unsupported) Enumerate() ([]Node, error)   { return nil, ErrUnsupportedPlatform }
func (unsupported) Open(string) (Control, error) { return nil, ErrUnsupportedPlatform }
