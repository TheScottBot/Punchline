// Package repocheck enforces the architecture's import boundaries:
//
//   - Only the OS-facing packages may use syscall, unsafe, x/sys or CGo.
//   - The application core and its model packages never import desktop
//     integration or camera adapters/backends.
//   - Desktop integration never imports camera adapters or backends.
//   - Nothing imports networking packages (no telemetry, no network).
package repocheck

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const module = "github.com/thescottbot/punchline"

// osPackages may touch the operating system directly.
var osPackages = map[string]bool{
	"internal/desktop": true,
	"internal/uvc":     true,
}

var osImports = []string{"syscall", "unsafe", "C", "golang.org/x/sys"}

var networkImports = []string{"net", "crypto/tls"}

// pure packages: application logic and models.
var pure = []string{
	"internal/app", "internal/camera", "internal/camera/mock", "internal/zoom", "internal/hotkey",
	"internal/settings", "internal/icon", "internal/eventlog", "internal/product",
}

var forbiddenForPure = []string{"internal/desktop", "internal/uvc", "internal/camera/"}

var forbiddenForDesktop = []string{"internal/uvc", "internal/camera/"}

// matchesAny: entries ending in "/" match strict subpackages only (so
// "internal/camera/" forbids adapters but not the camera interface);
// other entries match the package and its subpackages.
func matchesAny(local string, list []string) bool {
	for _, b := range list {
		if strings.HasSuffix(b, "/") {
			if strings.HasPrefix(local, b) {
				return true
			}
		} else if hasPrefixPath(local, b) {
			return true
		}
	}
	return false
}

func hasPrefixPath(imp, p string) bool {
	return imp == p || strings.HasPrefix(imp, p+"/")
}

// Check scans root and returns one message per violation.
func Check(root string) ([]string, error) {
	var out []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			n := d.Name()
			if path != root && (strings.HasPrefix(n, ".") || n == "testdata" || n == "vendor" || n == "third_party") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		pkg := filepath.ToSlash(rel)
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		file := filepath.ToSlash(mustRel(root, path))
		test := strings.HasSuffix(path, "_test.go")
		for _, is := range f.Imports {
			imp, _ := strconv.Unquote(is.Path.Value)
			local := strings.TrimPrefix(imp, module+"/")
			isLocal := local != imp
			for _, n := range networkImports {
				if hasPrefixPath(imp, n) {
					out = append(out, fmt.Sprintf("%s: network import %q", file, imp))
				}
			}
			if !osPackages[pkg] && !test {
				for _, o := range osImports {
					if hasPrefixPath(imp, o) {
						out = append(out, fmt.Sprintf("%s: OS import %q outside OS-facing packages", file, imp))
					}
				}
			}
			if !isLocal {
				continue
			}
			mockInTest := test && local == "internal/camera/mock"
			for _, p := range pure {
				if pkg == p && matchesAny(local, forbiddenForPure) && !mockInTest {
					out = append(out, fmt.Sprintf("%s: core package imports %q", file, imp))
				}
			}
			if pkg == "internal/desktop" && matchesAny(local, forbiddenForDesktop) && !mockInTest {
				out = append(out, fmt.Sprintf("%s: desktop imports camera implementation %q", file, imp))
			}
		}
		return nil
	})
	sort.Strings(out)
	return out, err
}

func mustRel(root, p string) string {
	r, err := filepath.Rel(root, p)
	if err != nil {
		return p
	}
	return r
}
