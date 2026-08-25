package farm

import (
	"image"
	"image/png"
	"io"
)

// PNGOptions is how the farm is written as a still raster picture.
type PNGOptions struct {
	Theme      *Theme
	Cols, Rows int // terminal cells; the canvas is Cols wide and Rows*2 pixels tall
	Scale      int // PNG pixels per canvas pixel
	Night      bool
}

func (o PNGOptions) normalise() PNGOptions {
	if o.Cols < MinCols {
		o.Cols = 120
	}
	if o.Rows < MinRows {
		o.Rows = 50
	}
	if o.Scale < 1 {
		o.Scale = 10
	}
	return o
}

func (o PNGOptions) theme() *Theme {
	if o.Theme == nil {
		return &Quiet
	}
	return o.Theme
}

// WritePNG draws the farm as a still raster picture.
//
// The SVG is the better file in every way that matters — it is smaller, it
// stays sharp at any size, and its names are real text. This exists for the
// places that will not take one: a social preview card, a slide, a chat window
// that pastes an image. All of those want a flat rectangle of pixels.
//
// So it is the file themes' picture, rasterised: the same leafier plants the
// SVG grows, on a canvas the same shape, which is why it shares writeSVG's
// fifty rows rather than a terminal's thirty-six.
//
// It carries the GIF's limitation, and for the same reason. The rune overlay —
// the hairline fences of the quiet themes, and every directory label — is
// characters, which a terminal draws and an SVG turns into <text>. Pixels carry
// neither. A PNG therefore has no names on its fields, and only the painted
// theme's fence survives the trip, because that one is pixel art already. A
// caller that has not asked for a theme should hand this Full.
func (s *Scene) WritePNG(w io.Writer, o PNGOptions) error {
	o = o.normalise()
	t := forFile(o.theme())

	c := NewCanvas(o.Cols, o.Rows*2)
	s.Draw(c, Options{
		Theme: t,
		Night: o.Night,
		Names: false, // the overlay is characters, and this is pixels
	})

	img := image.NewRGBA(image.Rect(0, 0, o.Cols*o.Scale, o.Rows*2*o.Scale))
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			px := c.At(x, y)
			if px.A == 0 {
				continue // transparent: NewRGBA is already zeroed
			}
			// One canvas pixel is a square of PNG pixels, filled in by hand
			// rather than by scaling an image, because a resampler would blur
			// the edges that make this pixel art.
			for dy := 0; dy < o.Scale; dy++ {
				for dx := 0; dx < o.Scale; dx++ {
					img.SetRGBA(x*o.Scale+dx, y*o.Scale+dy, px)
				}
			}
		}
	}
	return png.Encode(w, img)
}
