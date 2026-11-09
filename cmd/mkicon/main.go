// Command mkicon writes the macOS application icon set, drawn by
// internal/icon (the same magnifier as the menu bar icon). The install script
// turns it into AppIcon.icns with iconutil:
//
//	mkicon -out AppIcon.iconset && iconutil -c icns AppIcon.iconset
package main

import (
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/thescottbot/punchline/internal/icon"
)

// iconset lists the files iconutil expects: each point size at 1x and 2x.
var iconset = []struct {
	name string
	px   int
}{
	{"icon_16x16.png", 16}, {"icon_16x16@2x.png", 32},
	{"icon_32x32.png", 32}, {"icon_32x32@2x.png", 64},
	{"icon_128x128.png", 128}, {"icon_128x128@2x.png", 256},
	{"icon_256x256.png", 256}, {"icon_256x256@2x.png", 512},
	{"icon_512x512.png", 512}, {"icon_512x512@2x.png", 1024},
}

func main() {
	out := flag.String("out", "AppIcon.iconset", "directory to write (created; must end in .iconset for iconutil)")
	flag.Parse()
	if err := write(*out); err != nil {
		fmt.Fprintln(os.Stderr, "mkicon:", err)
		os.Exit(1)
	}
}

func write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, f := range iconset {
		w, err := os.Create(filepath.Join(dir, f.name))
		if err != nil {
			return err
		}
		err = png.Encode(w, icon.AppIcon(f.px))
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("%s: %w", f.name, err)
		}
	}
	return nil
}
