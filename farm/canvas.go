// Package farm draws a repository as pixel art, using half-block characters.
//
// A terminal cell is about twice as tall as it is wide. The character U+2580
// ("▀", upper half block) paints the top half of the cell in the foreground
// colour and leaves the bottom half showing the background colour. So one cell
// carries two square-ish pixels, and a 120x36 window becomes a 120x72 screen.
//
// The drawing layer came from git-farm-demo, where it was worked out against a
// made-up repository. What is new here is that the fields, the plants and the
// fences are what the history actually says.
package farm

import (
	"image/color"
	"math"
	"strings"
)

// Transparent pixels are skipped by Set, so whatever was drawn earlier shows
// through. Sprites use it for the space around the character.
var Transparent = color.RGBA{}

type Canvas struct {
	W, H int // in pixels; H is always even, two pixels per terminal row
	px   []color.RGBA

	// A character overlay, one entry per terminal cell rather than per pixel.
	//
	// A pixel is half a cell tall but a whole cell wide, so the thinnest line
	// the pixel grid can draw is still as wide as a character. A box-drawing
	// glyph is not: "│" is about an eighth of a cell wide. Where a hairline
	// needs to be thinner than a pixel, a cell can be handed over to a
	// character instead, and it wins by a factor of eight. Field labels use the
	// same layer.
	runes []rune
	runeF []color.RGBA
}

func NewCanvas(w, h int) *Canvas {
	if w < 1 {
		w = 1
	}
	if h < 2 {
		h = 2
	}
	h -= h % 2
	return &Canvas{
		W: w, H: h,
		px:    make([]color.RGBA, w*h),
		runes: make([]rune, w*h/2),
		runeF: make([]color.RGBA, w*h/2),
	}
}

// SetRune hands one terminal cell over to a character. cellY counts rows, not
// pixels, so a cell at cellY covers pixels 2*cellY and 2*cellY+1 — whatever was
// drawn in those two pixels is replaced.
func (c *Canvas) SetRune(x, cellY int, r rune, fg color.RGBA) {
	if x < 0 || cellY < 0 || x >= c.W || cellY >= c.H/2 {
		return
	}
	c.runes[cellY*c.W+x] = r
	c.runeF[cellY*c.W+x] = fg
}

// SetText writes a string into the character overlay, left to right. It stops
// at the edge of the canvas rather than wrapping: a label that does not fit is
// the caller's problem to shorten.
func (c *Canvas) SetText(x, cellY int, s string, fg color.RGBA) {
	for i, r := range []rune(s) {
		c.SetRune(x+i, cellY, r, fg)
	}
}

func (c *Canvas) Set(x, y int, col color.RGBA) {
	if col.A == 0 || x < 0 || y < 0 || x >= c.W || y >= c.H {
		return
	}
	c.px[y*c.W+x] = col
}

func (c *Canvas) At(x, y int) color.RGBA {
	if x < 0 || y < 0 || x >= c.W || y >= c.H {
		return Transparent
	}
	return c.px[y*c.W+x]
}

func (c *Canvas) Fill(col color.RGBA) {
	for i := range c.px {
		c.px[i] = col
	}
}

func (c *Canvas) Rect(x, y, w, h int, col color.RGBA) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			c.Set(x+dx, y+dy, col)
		}
	}
}

func (c *Canvas) Disc(cx, cy, r int, col color.RGBA) {
	for dy := -r; dy <= r; dy++ {
		for dx := -r; dx <= r; dx++ {
			if dx*dx+dy*dy <= r*r {
				c.Set(cx+dx, cy+dy, col)
			}
		}
	}
}

// Ring is a Disc with the middle left alone: a one-pixel outline. A filled
// shape would be a hole punched in a transparent picture, so the quiet theme
// draws its sun and moon this way.
func (c *Canvas) Ring(cx, cy, r int, col color.RGBA) {
	c.arc(cx, cy, r, col, func(int, int) bool { return false })
}

// Crescent is a Ring with the part covered by a second circle removed. The
// second circle has the same radius and is offset by (ox, oy). Equal radii
// matter: a smaller cutting circle leaves stray pixels at the top and bottom of
// the arc, which at this size look like dirt rather than a moon.
func (c *Canvas) Crescent(cx, cy, r, ox, oy int, col color.RGBA) {
	c.arc(cx, cy, r, col, func(x, y int) bool {
		return math.Hypot(float64(x-(cx+ox)), float64(y-(cy+oy))) <= float64(r)
	})
}

// arc keeps the pixels whose distance from the centre is close to r.
//
// The obvious integer version — inside r*r but outside (r-1)*(r-1) — leaves
// holes, because at a circle this small the gap between those two bands is
// wider than a pixel in some directions and narrower in others. Measuring the
// real distance and keeping a fixed band around r gives an even outline.
func (c *Canvas) arc(cx, cy, r int, col color.RGBA, skip func(x, y int) bool) {
	const band = 0.45
	for dy := -r - 1; dy <= r+1; dy++ {
		for dx := -r - 1; dx <= r+1; dx++ {
			if math.Abs(math.Hypot(float64(dx), float64(dy))-float64(r)) > band {
				continue
			}
			x, y := cx+dx, cy+dy
			if skip(x, y) {
				continue
			}
			c.Set(x, y, col)
		}
	}
}

// Render turns the pixel buffer into one string of escape codes.
//
// Three things are going on.
//
// First, colour codes are only written when the colour actually changes from
// the previous cell. A flat run of sky then costs 3 bytes per cell instead of
// 41, which removes about three quarters of the output.
//
// Second, transparency. A cell can hold two pixels, and either of them may be
// unpainted. "▀" fills the top half with the foreground and leaves the bottom
// half as the background, so:
//
//	both painted    ▀  fg = top, bg = bottom
//	only the top    ▀  fg = top, bg = the terminal's own background
//	only the bottom ▄  fg = bottom, bg = the terminal's own background
//	neither         a space, both left alone
//
// Reaching for the lower half block in the third case is the whole trick. It
// means a sprite can sit directly on the session with nothing painted behind it.
//
// Third, the profile. With no colour at all there is no foreground and
// background to play with, so a cell with both pixels painted becomes a full
// block and the picture falls back to one bit — which still carries the
// meaning, because the sprites are held apart by shape and ink before colour.
func (c *Canvas) Render(p Profile) string {
	var b strings.Builder
	b.Grow(c.W * c.H / 2 * 8)

	for y := 0; y < c.H; y += 2 {
		st := sgrState{profile: p}
		for x := 0; x < c.W; x++ {
			if r := c.runes[(y/2)*c.W+x]; r != 0 {
				// A character cell. The background stays whatever the lower
				// pixel was, so an overlay over painted ground still sits on it.
				st.fg(&b, c.runeF[(y/2)*c.W+x])
				if bot := c.At(x, y+1); bot.A != 0 {
					st.bg(&b, bot)
				} else {
					st.bgDefault(&b)
				}
				b.WriteRune(r)
				continue
			}

			top, bot := c.At(x, y), c.At(x, y+1)

			switch {
			case top.A != 0 && bot.A != 0:
				if p == Mono {
					b.WriteString("█")
					continue
				}
				st.fg(&b, top)
				st.bg(&b, bot)
				b.WriteString("▀")
			case top.A != 0:
				st.fg(&b, top)
				st.bgDefault(&b)
				b.WriteString("▀")
			case bot.A != 0:
				st.fg(&b, bot)
				st.bgDefault(&b)
				b.WriteString("▄")
			default:
				st.bgDefault(&b)
				b.WriteByte(' ')
			}
		}
		st.reset(&b)
		if y+2 < c.H {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// sgrState remembers what the terminal is already set to, so a colour code is
// only written when it would actually change something.
type sgrState struct {
	profile        Profile
	haveFg, haveBg bool
	curFg, curBg   color.RGBA
	defaultBg      bool
	wrote          bool
}

func (s *sgrState) fg(b *strings.Builder, col color.RGBA) {
	if s.profile == Mono || (s.haveFg && s.curFg == col) {
		return
	}
	s.profile.sgr(b, 38, col)
	s.haveFg, s.curFg, s.wrote = true, col, true
}

func (s *sgrState) bg(b *strings.Builder, col color.RGBA) {
	if s.profile == Mono || (s.haveBg && s.curBg == col && !s.defaultBg) {
		return
	}
	s.profile.sgr(b, 48, col)
	s.haveBg, s.curBg, s.defaultBg, s.wrote = true, col, false, true
}

func (s *sgrState) bgDefault(b *strings.Builder) {
	if s.profile == Mono || s.defaultBg {
		return
	}
	b.WriteString("\x1b[49m") // back to whatever the terminal was using
	s.defaultBg, s.haveBg, s.wrote = true, false, true
}

// reset ends a row, but only if that row wrote a colour code at all. Without
// the check, a monochrome frame would still be full of escape codes.
func (s *sgrState) reset(b *strings.Builder) {
	if s.wrote {
		b.WriteString("\x1b[0m")
	}
}
