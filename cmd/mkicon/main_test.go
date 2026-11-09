package main

import (
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteIconset(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "AppIcon.iconset")
	if err := write(dir); err != nil {
		t.Fatal(err)
	}
	for _, f := range iconset {
		r, err := os.Open(filepath.Join(dir, f.name))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := png.DecodeConfig(r)
		r.Close()
		if err != nil || cfg.Width != f.px || cfg.Height != f.px {
			t.Errorf("%s: %v %dx%d", f.name, err, cfg.Width, cfg.Height)
		}
	}
}
