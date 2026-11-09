package icon

import (
	"image/color"
	"math"
	"testing"
)

func transparent(c color.NRGBA) bool { return c.A == 0 }

func px(b []byte, size, x, y int) (bl, g, r, a byte) {
	i := (y*size + x) * 4
	return b[i], b[i+1], b[i+2], b[i+3]
}

func TestRender(t *testing.T) {
	for _, size := range []int{16, 20, 24, 32, 64} {
		b := Render(size, Active)
		if len(b) != size*size*4 {
			t.Fatalf("len %d", len(b))
		}
		if _, _, _, a := px(b, size, 0, 0); a != 0 {
			t.Errorf("%d: rounded corner not transparent", size)
		}
		// Inside the lens: the dark panel shows through, fully opaque.
		if bl, g, r, a := px(b, size, size*44/100, size*44/100); a != 255 || r > 0x30 || g > 0x30 || bl > 0x40 {
			t.Errorf("%d: lens centre not dark panel: %d %d %d %d", size, r, g, bl, a)
		}
		// Edge midpoint: the pink outline.
		if _, g, r, a := px(b, size, size/2, 0); a == 0 || r < 0x80 || g > r {
			t.Errorf("%d: outline missing at top edge", size)
		}
	}
}

func TestStylesDistinct(t *testing.T) {
	const s = 32
	// A pixel on the handle.
	_, ga, ra, _ := px(Render(s, Active), s, 21, 21)
	_, _, ri, _ := px(Render(s, Inactive), s, 21, 21)
	if ra < 0xe0 || ga > 0xa0 {
		t.Errorf("active glyph not pink: r %x g %x", ra, ga)
	}
	if ri > 0x90 {
		t.Errorf("inactive glyph not muted: r %x", ri)
	}
}

func TestImageMatchesRender(t *testing.T) {
	const s = 32
	img, b := Image(s, Active), Render(s, Active)
	for _, p := range [][2]int{{0, 0}, {16, 0}, {14, 14}, {21, 21}} {
		bl, g, r, a := px(b, s, p[0], p[1])
		c := img.NRGBAAt(p[0], p[1])
		if c.R != r || c.G != g || c.B != bl || c.A != a {
			t.Errorf("%v: image %v, render %d %d %d %d", p, c, r, g, bl, a)
		}
	}
}

func TestAppIconInsetInMargin(t *testing.T) {
	for _, size := range []int{16, 32, 128, 512, 1024} {
		img := AppIcon(size)
		if img.Bounds().Dx() != size || img.Bounds().Dy() != size {
			t.Fatalf("%d: bounds %v", size, img.Bounds())
		}
		margin := size * (appCanvas - appTile) / 2 / appCanvas
		if margin > 0 && !transparent(img.NRGBAAt(margin-1, size/2)) {
			t.Errorf("%d: margin not transparent", size)
		}
		// The tile's top edge midpoint carries the pink outline.
		if c := img.NRGBAAt(size/2, margin+1); c.A == 0 || c.R < 0x80 {
			t.Errorf("%d: outline missing at tile top: %v", size, c)
		}
		// The lens centre is the opaque dark panel.
		if c := img.NRGBAAt(margin+(size-2*margin)*44/100, margin+(size-2*margin)*44/100); c.A != 255 || c.R > 0x30 {
			t.Errorf("%d: lens centre %v", size, c)
		}
	}
	if AppIcon(1).Bounds().Dx() != 1 {
		t.Error("tiny size")
	}
}

// Nothing of the glyph may fall inside the lens: every pixel whose square
// lies wholly within the ring's inner edge must be the plain dark panel.
func TestLensInteriorClear(t *testing.T) {
	for _, size := range []int{16, 18, 32, 36, 64, 256, 1024} {
		s := float64(size)
		cx, cy := 0.44*s, 0.44*s
		ring := math.Max(1.3, 0.08*s)
		inner := 0.19*s - ring/2
		b := Render(size, Active)
		for y := 0; y < size; y++ {
			for x := 0; x < size; x++ {
				// Farthest corner of the pixel from the lens centre.
				dx := math.Max(math.Abs(float64(x)-cx), math.Abs(float64(x+1)-cx))
				dy := math.Max(math.Abs(float64(y)-cy), math.Abs(float64(y+1)-cy))
				if math.Hypot(dx, dy) >= inner {
					continue
				}
				if bl, g, r, a := px(b, size, x, y); a != 255 || r != 0x12 || g != 0x13 || bl != 0x1c {
					t.Fatalf("%d: glyph inside the lens at (%d,%d): %02x%02x%02x a%d", size, x, y, r, g, bl, a)
				}
			}
		}
	}
}
