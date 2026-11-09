package icon

import "testing"

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
