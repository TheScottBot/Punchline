package icon

import "image"

// Image returns the icon as an image (straight alpha), for writing files.
func Image(size int, st Style) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	b := Render(size, st)
	for i := 0; i+3 < len(b); i += 4 {
		// Render is B, G, R, A; NRGBA is R, G, B, A.
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = b[i+2], b[i+1], b[i], b[i+3]
	}
	return img
}

// App icon geometry from Apple's macOS icon grid: on a 1024 canvas the
// rounded tile is 824 wide, leaving a 100 px transparent margin, so the icon
// sits at the same size as other apps' icons in Finder and the Dock.
const (
	appCanvas = 1024
	appTile   = 824
)

// AppIcon returns the application icon at size×size: the armed (pink)
// menu bar icon, inset within the standard transparent margin.
func AppIcon(size int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	tile := size * appTile / appCanvas
	if tile < 1 {
		return img
	}
	off := (size - tile) / 2
	src := Image(tile, Active)
	for y := 0; y < tile; y++ {
		for x := 0; x < tile; x++ {
			img.SetNRGBA(off+x, off+y, src.NRGBAAt(x, y))
		}
	}
	return img
}
