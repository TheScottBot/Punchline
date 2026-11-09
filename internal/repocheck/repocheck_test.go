package repocheck

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRepositoryBoundaries(t *testing.T) {
	v, err := Check(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range v {
		t.Error(m)
	}
}

func TestDetectsViolations(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		p := filepath.Join(root, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		os.WriteFile(p, []byte(src), 0o644)
	}
	write("internal/app/a.go", `package app
import (_ "github.com/thescottbot/punchline/internal/desktop"; _ "unsafe"; _ "net/http")`)
	write("internal/desktop/d.go", `package desktop
import _ "github.com/thescottbot/punchline/internal/camera/streamcam"`)
	v, err := Check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 4 {
		t.Fatalf("got %d: %q", len(v), v)
	}
}
