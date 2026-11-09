// Package icon draws the tray / menu bar icon into a pixel buffer: a
// magnifying glass on a dark rounded tile with a neon outline, in Thyme's
// palette. Pink when armed, muted when idle. Pure Go; the desktop package
// turns the buffer into a native icon.
package icon

import "math"

// Style selects the icon's colours.
type Style int

const (
	Active   Style = iota // hotkeys enabled and camera connected
	Inactive              // disabled, or no camera
)

type rgb struct{ r, g, b float64 }

func hex(v uint32) rgb {
	return rgb{float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255}
}

// Thyme's theme: PanelFill, Outline, TimeText.
var (
	panelFill = hex(0x12131c)
	palette   = map[Style][2]rgb{
		//        outline          glyph
		Active:   {hex(0xf59ef3), hex(0xf894f8)},
		Inactive: {hex(0x5c4f63), hex(0x6e6175)},
	}
)

// Render returns size*size pixels, top-down rows, 4 bytes per pixel in
// B, G, R, A order with straight (non-premultiplied) alpha.
func Render(size int, st Style) []byte {
	out := make([]byte, size*size*4)
	if size <= 0 {
		return out
	}
	pal, ok := palette[st]
	if !ok {
		pal = palette[Inactive]
	}
	s := float64(size)
	half := s/2 - 0.5
	corner := 0.22 * s
	outline := math.Max(1, s/16)

	cx, cy := 0.44*s, 0.44*s
	rad := 0.19 * s
	ring := math.Max(1.3, 0.08*s)
	handle := math.Max(1.7, 0.11*s)
	// The handle starts half its width outside the ring's centre line, so its
	// rounded end reaches the ring but never crosses into the lens.
	hstart := rad + handle/2
	hx0, hy0 := cx+hstart*0.7071, cy+hstart*0.7071
	hx1, hy1 := 0.73*s, 0.73*s

	const ss = 4
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			var ar, ag, ab, aa float64
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					px := float64(x) + (float64(sx)+0.5)/ss
					py := float64(y) + (float64(sy)+0.5)/ss
					dTile := roundRect(px-s/2, py-s/2, half, corner)
					if dTile > 0 {
						continue
					}
					c := panelFill
					if dTile > -outline {
						c = pal[0]
					}
					dGlyph := math.Min(
						math.Abs(math.Hypot(px-cx, py-cy)-rad)-ring/2,
						segDist(px, py, hx0, hy0, hx1, hy1)-handle/2)
					if dGlyph <= 0 {
						c = pal[1]
					}
					ar += c.r
					ag += c.g
					ab += c.b
					aa++
				}
			}
			i := (y*size + x) * 4
			if aa > 0 {
				out[i+0] = byte(math.Round(ab / aa * 255))
				out[i+1] = byte(math.Round(ag / aa * 255))
				out[i+2] = byte(math.Round(ar / aa * 255))
			}
			out[i+3] = byte(math.Round(aa / (ss * ss) * 255))
		}
	}
	return out
}

// roundRect is the signed distance from (x, y), relative to the centre, to a
// square of half-size h with corner radius r.
func roundRect(x, y, h, r float64) float64 {
	qx, qy := math.Abs(x)-(h-r), math.Abs(y)-(h-r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

func segDist(px, py, x0, y0, x1, y1 float64) float64 {
	dx, dy := x1-x0, y1-y0
	t := ((px-x0)*dx + (py-y0)*dy) / (dx*dx + dy*dy)
	t = math.Max(0, math.Min(1, t))
	return math.Hypot(px-(x0+t*dx), py-(y0+t*dy))
}
