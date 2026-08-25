package farm

import (
	"image"
	"image/color"
	"image/gif"
	"io"
	"sort"
)

// GIFOptions is how a history is written as a moving picture.
type GIFOptions struct {
	Theme      *Theme
	Cols, Rows int // terminal cells; the canvas is Cols wide and Rows*2 pixels tall
	Scale      int // GIF pixels per canvas pixel
	Names      bool

	// Delay is how long an ordinary frame is held, in hundredths of a second,
	// which is the only unit a GIF has.
	Delay int

	// Hold is how long the last frame stays before the loop restarts. A
	// time-lapse that snaps back the instant it arrives never lets anybody see
	// where it got to, which is the frame they actually came for.
	Hold int
}

func (o GIFOptions) normalise() GIFOptions {
	if o.Cols < MinCols {
		o.Cols = 120
	}
	if o.Rows < MinRows {
		o.Rows = 40
	}
	if o.Scale < 1 {
		o.Scale = 4
	}
	if o.Delay < 2 {
		o.Delay = 12
	}
	if o.Hold < o.Delay {
		o.Hold = 250
	}
	return o
}

func (o GIFOptions) theme() *Theme {
	if o.Theme == nil {
		return &Quiet
	}
	return o.Theme
}

// A FrameFunc paints one frame onto the canvas. The caller owns what a frame
// means; this package owns turning canvases into a file.
type FrameFunc func(i int, c *Canvas)

// WriteGIF paints n frames and writes them as one looping image.
//
// A GIF has a palette of at most 256 colours, and the farm has nowhere near
// that many: seven roles in the quiet themes, and a sky gradient in the
// painted one that is a couple of dozen shades of the same blue. So the palette
// is built from the colours the frames actually use rather than quantised down
// from a general one, and every pixel lands on its exact colour.
//
// Transparent is index 0 in every frame, so a quiet farm keeps its transparency
// and sits on whatever the page behind it is.
func WriteGIF(w io.Writer, n int, o GIFOptions, paint FrameFunc) error {
	o = o.normalise()

	canvases := make([]*Canvas, 0, n)
	seen := map[color.RGBA]bool{Transparent: true}

	for i := 0; i < n; i++ {
		c := NewCanvas(o.Cols, o.Rows*2)
		paint(i, c)
		canvases = append(canvases, c)

		for _, px := range c.px {
			if px.A == 0 {
				px = Transparent
			}
			seen[px] = true
		}
	}

	pal, index := palette(seen)
	out := &gif.GIF{LoopCount: 0}

	for i, c := range canvases {
		img := image.NewPaletted(image.Rect(0, 0, o.Cols*o.Scale, o.Rows*2*o.Scale), pal)

		for y := 0; y < c.H; y++ {
			for x := 0; x < c.W; x++ {
				px := c.At(x, y)
				if px.A == 0 {
					px = Transparent
				}
				idx := index[px]
				if idx == 0 {
					continue // transparent: the frame is already zeroed
				}
				// One canvas pixel is a square of GIF pixels. Written by hand
				// rather than by scaling an image, because a resampler would
				// blur the edges that make this pixel art.
				for dy := 0; dy < o.Scale; dy++ {
					row := (y*o.Scale + dy) * img.Stride
					for dx := 0; dx < o.Scale; dx++ {
						img.Pix[row+x*o.Scale+dx] = idx
					}
				}
			}
		}

		delay := o.Delay
		if i == len(canvases)-1 {
			delay = o.Hold
		}
		out.Image = append(out.Image, img)
		out.Delay = append(out.Delay, delay)
		out.Disposal = append(out.Disposal, gif.DisposalBackground)
	}

	if len(out.Image) == 0 {
		return nil
	}
	out.Config = image.Config{
		ColorModel: pal,
		Width:      o.Cols * o.Scale,
		Height:     o.Rows * 2 * o.Scale,
	}
	out.BackgroundIndex = 0
	return gif.EncodeAll(w, out)
}

// palette turns the colours the frames used into a GIF palette, with
// transparent first.
//
// Sorted, not in map order, because two runs over the same repository have to
// produce the same bytes — the same rule the SVG follows, and for the same
// reason: a file that changes when nothing changed is a file that gets pushed
// every night.
func palette(seen map[color.RGBA]bool) (color.Palette, map[color.RGBA]uint8) {
	cols := make([]color.RGBA, 0, len(seen))
	for c := range seen {
		if c == Transparent {
			continue
		}
		cols = append(cols, c)
	}
	sort.Slice(cols, func(i, j int) bool { return rgbKey(cols[i]) < rgbKey(cols[j]) })

	// 255 and not 256: index 0 is spoken for.
	if len(cols) > 255 {
		cols = cols[:255]
	}

	pal := color.Palette{color.RGBA{}} // index 0, fully transparent
	index := map[color.RGBA]uint8{Transparent: 0}
	for i, c := range cols {
		pal = append(pal, c)
		index[c] = uint8(i + 1)
	}
	return pal, index
}

func rgbKey(c color.RGBA) uint32 {
	return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}
