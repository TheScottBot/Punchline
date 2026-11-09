// Package zoom maps logical magnification onto device-specific absolute
// zoom values. Adapters use it; the application core never sees raw values.
package zoom

import "math"

// Range is a device's absolute zoom control range.
type Range struct {
	Min, Max, Step, Default int32
}

// Valid reports whether the range describes a usable zoom control.
func (r Range) Valid() bool { return r.Max > r.Min }

// Clamp limits mag to [1, maxMag]. A maxMag below 1 is treated as 1.
func Clamp(mag, maxMag float64) float64 {
	if maxMag < 1 || math.IsNaN(maxMag) {
		maxMag = 1
	}
	if math.IsNaN(mag) || mag < 1 {
		return 1
	}
	if mag > maxMag {
		return maxMag
	}
	return mag
}

// ToRaw maps mag (1 = Min, maxMag = Max) linearly onto r, aligned to Step
// from Min and clamped to the range.
func ToRaw(mag, maxMag float64, r Range) int32 {
	if !r.Valid() || maxMag <= 1 {
		return r.Min
	}
	mag = Clamp(mag, maxMag)
	span := float64(r.Max - r.Min)
	raw := float64(r.Min) + (mag-1)/(maxMag-1)*span
	step := float64(r.Step)
	if step > 0 {
		raw = float64(r.Min) + math.Round((raw-float64(r.Min))/step)*step
	} else {
		raw = math.Round(raw)
	}
	if raw > float64(r.Max) {
		raw = float64(r.Max)
	}
	if raw < float64(r.Min) {
		raw = float64(r.Min)
	}
	return int32(raw)
}

// FromRaw is the inverse of ToRaw.
func FromRaw(raw int32, maxMag float64, r Range) float64 {
	if !r.Valid() || maxMag <= 1 {
		return 1
	}
	if raw < r.Min {
		raw = r.Min
	}
	if raw > r.Max {
		raw = r.Max
	}
	return 1 + float64(raw-r.Min)/float64(r.Max-r.Min)*(maxMag-1)
}
