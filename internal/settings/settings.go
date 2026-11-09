// Package settings holds the persisted configuration: defaults, validation,
// load and atomic save. It has no OS-specific code.
package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/thescottbot/punchline/internal/hotkey"
	"github.com/thescottbot/punchline/internal/product"
)

// Version is the only settings format understood.
const Version = 1

// MaxMagnification bounds configured zoom levels.
const MaxMagnification = 16.0

const maxFileSize = 64 << 10

// FileName is the settings file's name within Dir.
const FileName = "settings.json"

// ZoomOverride replaces the global zoom levels for one camera. Zero means
// "use the global value".
type ZoomOverride struct {
	Normal  float64 `json:"normal,omitempty"`
	Halfway float64 `json:"halfway,omitempty"`
	CloseUp float64 `json:"closeUp,omitempty"`
}

// Settings is the persisted configuration.
type Settings struct {
	Version         int    `json:"version"`
	HotkeysEnabled  bool   `json:"hotkeysEnabled"`
	NormalKey       string `json:"normalKey"`
	HalfwayKey      string `json:"halfwayKey"`
	CloseUpKey      string `json:"closeUpKey"`
	PreferredCamera string `json:"preferredCamera"`
	// NormalZoom is the normal framing magnification.
	NormalZoom float64 `json:"normalZoom"`
	// HalfwayZoom is the halfway magnification; 0 means the camera model's
	// halfway level. Cameras without one never use the halfway hotkey.
	HalfwayZoom float64 `json:"halfwayZoom"`
	// CloseUpZoom is the close-up magnification; 0 means the camera maximum.
	CloseUpZoom float64 `json:"closeUpZoom"`
	// CameraZoom holds per-camera overrides keyed by camera ID.
	CameraZoom map[string]ZoomOverride `json:"cameraZoom"`
}

// Defaults returns the configuration used without a settings file.
func Defaults() Settings {
	b := hotkey.DefaultBindings()
	return Settings{
		Version:        Version,
		HotkeysEnabled: true,
		NormalKey:      string(b.Normal),
		HalfwayKey:     string(b.Halfway),
		CloseUpKey:     string(b.CloseUp),
		NormalZoom:     1,
		CloseUpZoom:    0,
		CameraZoom:     map[string]ZoomOverride{},
	}
}

// Bindings returns the hotkey bindings.
func (s Settings) Bindings() hotkey.Bindings {
	return hotkey.Bindings{Normal: hotkey.Key(s.NormalKey), Halfway: hotkey.Key(s.HalfwayKey), CloseUp: hotkey.Key(s.CloseUpKey)}
}

// WithBindings returns a copy with the given bindings.
func (s Settings) WithBindings(b hotkey.Bindings) Settings {
	s.NormalKey, s.HalfwayKey, s.CloseUpKey = string(b.Normal), string(b.Halfway), string(b.CloseUp)
	return s
}

// Clone returns a deep copy.
func (s Settings) Clone() Settings {
	m := make(map[string]ZoomOverride, len(s.CameraZoom))
	for k, v := range s.CameraZoom {
		m[k] = v
	}
	s.CameraZoom = m
	return s
}

// Dir returns the platform-appropriate per-user folder for settings and log:
// %LOCALAPPDATA%\Punchline on Windows, ~/Library/Application Support/Punchline
// on macOS.
func Dir() (string, error) {
	var base string
	var err error
	if runtime.GOOS == "windows" {
		base = os.Getenv("LOCALAPPDATA")
		if base == "" {
			base, err = os.UserCacheDir()
		}
	} else {
		base, err = os.UserConfigDir()
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(base, product.ShortName), nil
}

// Load reads path. A missing file yields defaults. Invalid fields are
// replaced by their defaults and reported in problems (by field and reason,
// never content). A file that cannot be understood at all is renamed to
// path+".invalid" so a later save does not destroy it.
func Load(path string) (s Settings, problems []string, err error) {
	s = Defaults()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil, nil
	}
	if err != nil {
		return s, nil, err
	}
	quarantine := func(reason string) (Settings, []string, error) {
		problems = append(problems, "file ignored: "+reason)
		if rerr := os.Rename(path, path+".invalid"); rerr != nil {
			problems = append(problems, "could not set invalid file aside: "+rerr.Error())
		} else {
			problems = append(problems, "invalid file kept as "+filepath.Base(path)+".invalid")
		}
		return Defaults(), problems, nil
	}
	if len(data) > maxFileSize {
		return quarantine("larger than 64 KiB")
	}
	var fields map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&fields); err != nil || fields == nil {
		return quarantine("not a JSON object")
	}
	var version int
	if raw, ok := fields["version"]; !ok || json.Unmarshal(raw, &version) != nil || version != Version {
		return quarantine(fmt.Sprintf("version is not %d", Version))
	}

	bad := func(field, reason string) { problems = append(problems, field+": "+reason+"; using default") }
	known := map[string]func(json.RawMessage){
		"version": func(json.RawMessage) {},
		"hotkeysEnabled": func(r json.RawMessage) {
			if json.Unmarshal(r, &s.HotkeysEnabled) != nil {
				s.HotkeysEnabled = Defaults().HotkeysEnabled
				bad("hotkeysEnabled", "not a boolean")
			}
		},
		"normalKey":  keyField(&s.NormalKey, "normalKey", bad),
		"halfwayKey": keyField(&s.HalfwayKey, "halfwayKey", bad),
		"closeUpKey": keyField(&s.CloseUpKey, "closeUpKey", bad),
		"preferredCamera": func(r json.RawMessage) {
			var v string
			if json.Unmarshal(r, &v) != nil || len(v) > 512 {
				bad("preferredCamera", "not a string")
				return
			}
			s.PreferredCamera = v
		},
		"normalZoom": func(r json.RawMessage) {
			var v float64
			if json.Unmarshal(r, &v) != nil || !validMag(v, false) {
				bad("normalZoom", fmt.Sprintf("must be between 1 and %g", MaxMagnification))
				return
			}
			s.NormalZoom = v
		},
		"halfwayZoom": func(r json.RawMessage) {
			var v float64
			if json.Unmarshal(r, &v) != nil || !validMag(v, true) {
				bad("halfwayZoom", fmt.Sprintf("must be 0 (camera's halfway level) or between 1 and %g", MaxMagnification))
				return
			}
			s.HalfwayZoom = v
		},
		"closeUpZoom": func(r json.RawMessage) {
			var v float64
			if json.Unmarshal(r, &v) != nil || !validMag(v, true) {
				bad("closeUpZoom", fmt.Sprintf("must be 0 (camera maximum) or between 1 and %g", MaxMagnification))
				return
			}
			s.CloseUpZoom = v
		},
		"cameraZoom": func(r json.RawMessage) {
			var v map[string]ZoomOverride
			if json.Unmarshal(r, &v) != nil {
				bad("cameraZoom", "not an object of {normal, halfway, closeUp}")
				return
			}
			for id, o := range v {
				if !validMag(o.Normal, true) || !validMag(o.Halfway, true) || !validMag(o.CloseUp, true) {
					problems = append(problems, "cameraZoom: an entry is out of range; entry ignored")
					delete(v, id)
				}
			}
			s.CameraZoom = v
		},
	}
	names := make([]string, 0, len(fields))
	for k := range fields {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		if f, ok := known[k]; ok {
			f(fields[k])
		} else {
			problems = append(problems, fmt.Sprintf("unknown field %q ignored", k))
		}
	}
	if s.CameraZoom == nil {
		s.CameraZoom = map[string]ZoomOverride{}
	}
	if s.NormalKey == s.CloseUpKey {
		problems = append(problems, "normalKey and closeUpKey are the same; using default bindings")
		d := Defaults()
		s.NormalKey, s.HalfwayKey, s.CloseUpKey = d.NormalKey, d.HalfwayKey, d.CloseUpKey
	}
	if s.HalfwayKey == s.NormalKey || s.HalfwayKey == s.CloseUpKey {
		// Typically a file from before the halfway level whose normal or
		// close-up key is F15: keep those, move halfway to a free key.
		problems = append(problems, "halfwayKey clashes with another binding; using a free F-key")
		s.HalfwayKey = freeKey(s.NormalKey, s.CloseUpKey)
	}
	return s, problems, nil
}

// freeKey returns the first of F15..F24, then F13..F14, not in used.
func freeKey(used ...string) string {
	for _, n := range []int{15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 13, 14} {
		k := fmt.Sprintf("F%d", n)
		clash := false
		for _, u := range used {
			clash = clash || u == k
		}
		if !clash {
			return k
		}
	}
	return Defaults().HalfwayKey
}

func keyField(dst *string, name string, bad func(string, string)) func(json.RawMessage) {
	return func(r json.RawMessage) {
		var v string
		if json.Unmarshal(r, &v) != nil {
			bad(name, "not a string")
			return
		}
		k, ok := hotkey.Parse(v)
		if !ok {
			bad(name, "not a supported key")
			return
		}
		*dst = string(k)
	}
}

func validMag(v float64, zeroOK bool) bool {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return false
	}
	if v == 0 {
		return zeroOK
	}
	return v >= 1 && v <= MaxMagnification
}

// Save writes s to path atomically.
func Save(path string, s Settings) error {
	s.Version = Version
	if s.CameraZoom == nil {
		s.CameraZoom = map[string]ZoomOverride{}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "settings-*.tmp")
	if err != nil {
		return err
	}
	_, werr := tmp.Write(data)
	cerr := tmp.Close()
	if werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp.Name())
		return werr
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// FileStore saves settings to a fixed path.
type FileStore string

// Save writes s to the store's path.
func (f FileStore) Save(s Settings) error { return Save(string(f), s) }
