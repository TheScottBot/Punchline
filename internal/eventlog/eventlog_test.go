package eventlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.log")
	l, err := Open(path, 100)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	line := strings.Repeat("a", 39) + "\n"
	for i := 0; i < 5; i++ {
		if _, err := l.Write([]byte(line)); err != nil {
			t.Fatal(err)
		}
	}
	cur, _ := os.ReadFile(path)
	old, _ := os.ReadFile(path + ".1")
	if len(cur) > 100 || len(old) == 0 || len(old) > 100 {
		t.Fatalf("cur %d old %d", len(cur), len(old))
	}
}
