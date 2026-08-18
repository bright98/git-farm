package farm

import (
	"image/color"
	"io"
	"sort"
	"strconv"
	"strings"
)

// SVGOptions is how the farm is written to a file rather than to a terminal.
type SVGOptions struct {
	Theme      *Theme
	Cols, Rows int // terminal cells; the canvas is Cols wide and Rows*2 pixels tall
	Scale      int // SVG units per canvas pixel
	Names      bool
	Night      bool

	// Animate walks the farmer across their field with CSS keyframes. Only the
	// farmer moves; the rest of the picture is a static background.
	Animate bool

	// Title is the accessible name of the image.
	Title string
}

func (o SVGOptions) theme() *Theme {
	if o.Theme == nil {
		return &Quiet
	}
	return o.Theme
}

func (o SVGOptions) normalise() SVGOptions {
	if o.Cols < MinCols {
		o.Cols = 120
	}
	if o.Rows < MinRows {
		o.Rows = 36
	}
	if o.Scale < 1 {
		o.Scale = 6
	}
	return o
}

// WriteSVG draws the farm as an SVG.
//
// Three things are different from the terminal, and all three are because SVG
// has what a terminal does not.
//
// The pixels are merged. Every row is cut into runs of one colour, runs that
// sit directly above each other are merged into rectangles, and every rectangle
// of one colour goes into a single <path>. A 120x72 farm is 8,640 pixels and
// comes out as about seventy elements.
//
// The fences and the names are drawn as real lines and real text rather than as
// characters. In a terminal a box-drawing glyph is the only thing thinner than
// a pixel; here a stroke can be any width, dashes are real dashes, and a name
// is a <text> element that stays sharp at any size. It is the one place in this
// project where pixel art and typography sit in the same picture.
//
// And there is no <script>, ever. GitHub strips it. The farmer walks on CSS
// keyframes over a background that never changes.
func (s *Scene) WriteSVG(w io.Writer, o SVGOptions) error {
	o = o.normalise()
	t := forFile(o.theme())

	c := NewCanvas(o.Cols, o.Rows*2)
	s.Draw(c, Options{
		Theme:    t,
		Night:    o.Night,
		Names:    o.Names,
		Vector:   true, // the fences and names are drawn below, as vectors
		NoFarmer: true, // the farmer is drawn last, so they can be animated
	})

	b := &strings.Builder{}
	width, height := o.Cols*o.Scale, o.Rows*2*o.Scale

	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="`)
	b.WriteString(strconv.Itoa(width))
	b.WriteString(`" height="`)
	b.WriteString(strconv.Itoa(height))
	b.WriteString(`" viewBox="0 0 `)
	b.WriteString(strconv.Itoa(width))
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(height))
	b.WriteString(`" role="img" aria-label="`)
	b.WriteString(escape(o.title()))
	b.WriteString("\">\n<title>")
	b.WriteString(escape(o.title()))
	b.WriteString("</title>\n")

	writeStyle(b, o, s.farmerSpan(t))
	writePixels(b, c, o.Scale)
	s.writeFields(b, t, o)
	s.writeFarmer(b, t, o)

	b.WriteString("</svg>\n")

	_, err := io.WriteString(w, b.String())
	return err
}

// forFile gives a theme built for a terminal the sprites it would have drawn
// if it had never had to share the screen with anything.
//
// The quiet themes draw four-pixel plants because they sit on top of a live
// session and must not cover it. A file has nothing behind it, and it is looked
// at from further away, so it gets the fuller set — which obeys the same rules
// about silhouette and ink, with more room to obey them in.
//
// A theme that paints its own world is left alone: its sprites are the picture.
func forFile(t *Theme) *Theme {
	if t.Border != BorderGlyph {
		return t
	}
	leafy := *t
	leafy.Sprites = svgSprites
	leafy.StepX, leafy.StepY = 7, 7
	return &leafy
}

func (o SVGOptions) title() string {
	if o.Title != "" {
		return o.Title
	}
	return "a repository drawn as a farm"
}

// ---- the pixels -------------------------------------------------------

// writePixels turns the pixel buffer into one path per colour.
func writePixels(b *strings.Builder, c *Canvas, scale int) {
	colours, rects := pixelRects(c)
	if len(colours) == 0 {
		return
	}

	b.WriteString("<g shape-rendering=\"crispEdges\">\n")
	for _, col := range colours {
		b.WriteString(`<path fill="`)
		b.WriteString(hex(col))
		b.WriteString(`" d="`)
		for _, r := range rects[col] {
			b.WriteByte('M')
			b.WriteString(strconv.Itoa(r.X * scale))
			b.WriteByte(' ')
			b.WriteString(strconv.Itoa(r.Y * scale))
			b.WriteByte('h')
			b.WriteString(strconv.Itoa(r.W * scale))
			b.WriteByte('v')
			b.WriteString(strconv.Itoa(r.H * scale))
			b.WriteByte('h')
			b.WriteString(strconv.Itoa(-r.W * scale))
			b.WriteByte('z')
		}
		b.WriteString("\"/>\n")
	}
	b.WriteString("</g>\n")
}

// pixelRects cuts the canvas into the fewest rectangles it can manage cheaply:
// runs along each row, then runs merged with the identical run above them.
//
// The vertical pass is worth its ten lines. Plant stems, fence posts and the
// sky gradient are all columns of the same colour, so it roughly halves the
// path data for nothing.
//
// The colours come back sorted, because a map's order is not the same twice and
// this file has to be byte-identical between runs.
func pixelRects(c *Canvas) ([]color.RGBA, map[color.RGBA][]Rect) {
	type run struct{ x, y, w int }

	runs := map[color.RGBA][]run{}
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; {
			col := c.At(x, y)
			if col.A == 0 {
				x++
				continue
			}
			n := 1
			for x+n < c.W && c.At(x+n, y) == col {
				n++
			}
			runs[col] = append(runs[col], run{x, y, n})
			x += n
		}
	}

	colours := make([]color.RGBA, 0, len(runs))
	for col := range runs {
		colours = append(colours, col)
	}
	sort.Slice(colours, func(i, j int) bool { return packed(colours[i]) < packed(colours[j]) })

	out := make(map[color.RGBA][]Rect, len(runs))
	for _, col := range colours {
		rs := runs[col]
		// Sort into columns of identical runs, so the merge is one pass.
		sort.Slice(rs, func(i, j int) bool {
			if rs[i].x != rs[j].x {
				return rs[i].x < rs[j].x
			}
			if rs[i].w != rs[j].w {
				return rs[i].w < rs[j].w
			}
			return rs[i].y < rs[j].y
		})

		var merged []Rect
		for _, r := range rs {
			if n := len(merged); n > 0 {
				last := &merged[n-1]
				if last.X == r.x && last.W == r.w && last.Y+last.H == r.y {
					last.H++
					continue
				}
			}
			merged = append(merged, Rect{X: r.x, Y: r.y, W: r.w, H: 1})
		}

		// Back into reading order, so the file is stable and diffable.
		sort.Slice(merged, func(i, j int) bool {
			if merged[i].Y != merged[j].Y {
				return merged[i].Y < merged[j].Y
			}
			return merged[i].X < merged[j].X
		})
		out[col] = merged
	}
	return colours, out
}

// ---- the fences and the names -----------------------------------------

// fontFor keeps one character of text the same width as one canvas pixel, which
// is what the terminal gets for free and what makes the labels line up with the
// picture. A monospace advance is about 0.6em, so the size is the pixel width
// divided by that.
func (o SVGOptions) fontSize() int { return o.Scale * 5 / 3 }

func (s *Scene) writeFields(b *strings.Builder, t *Theme, o SVGOptions) {
	scale := o.Scale
	stroke := maxInt(1, scale/3)

	// Only a theme that framed its fields with box-drawing characters gets a
	// vector fence here. A theme that built a wooden one out of pixels already
	// has its fence, in the pixels.
	vectorFence := t.Border == BorderGlyph

	var fences, labels strings.Builder

	for _, f := range s.Fields {
		r := f.Bounds()
		if r.W == 0 {
			continue
		}

		x0, y0 := r.X*scale, r.Y*scale
		x1, y1 := (r.X+r.W)*scale, (r.Y+r.H)*scale

		// The name sits in the top edge, and the edge breaks to let it through
		// — the same thing the terminal does by handing those cells to text.
		// The columns come from LabelSpan, so the hole and the name cannot
		// disagree about where they are.
		gapFrom, gapTo, label := 0, 0, ""
		if o.Names {
			from, to, text := LabelSpan(r, f.Name)
			gapFrom, gapTo, label = from*scale, to*scale, text
		}

		if !vectorFence {
			writeLabel(&labels, label, gapFrom, y0, o)
			continue
		}

		var d strings.Builder
		k := 3 * scale // the length of a corner stub, for a fence nobody can claim

		top := [][2]int{{x0, x1}}
		if f.Fence == FenceUnknown {
			top = [][2]int{{x0, minInt(x0+k, x1)}, {maxInt(x1-k, x0), x1}}
		}
		for _, seg := range top {
			for _, part := range cut(seg[0], seg[1], gapFrom, gapTo) {
				moveH(&d, part[0], y0, part[1])
			}
		}

		if f.Fence == FenceUnknown {
			// Crop marks: the corners, and nothing between them. The field is
			// still one legible rectangle and no claim has been made about it.
			moveV(&d, x0, y0, y0+k)
			moveV(&d, x1, y0, y0+k)
			moveH(&d, x0, y1, x0+k)
			moveH(&d, x1-k, y1, x1)
			moveV(&d, x0, y1-k, y1)
			moveV(&d, x1, y1-k, y1)
		} else {
			moveV(&d, x0, y0, y1)
			moveV(&d, x1, y0, y1)
			moveH(&d, x0, y1, x1)
		}

		fences.WriteString(`<path class="fence`)
		if f.Fence == FenceBroken {
			fences.WriteString(" broken")
		}
		fences.WriteString(`" stroke="`)
		fences.WriteString(hex(t.Colour(RoleFence)))
		fences.WriteString(`" stroke-width="`)
		fences.WriteString(strconv.Itoa(stroke))
		fences.WriteString(`" d="`)
		fences.WriteString(d.String())
		fences.WriteString("\"/>\n")

		writeLabel(&labels, label, gapFrom, y0, o)
	}

	if fences.Len() > 0 {
		b.WriteString("<g fill=\"none\" stroke-linecap=\"butt\">\n")
		b.WriteString(fences.String())
		b.WriteString("</g>\n")
	}
	if labels.Len() > 0 {
		b.WriteString(`<g class="label" fill="`)
		b.WriteString(hex(t.Colour(RoleLabel)))
		b.WriteString(`" font-size="`)
		b.WriteString(strconv.Itoa(o.fontSize()))
		b.WriteString("\">\n")
		b.WriteString(labels.String())
		b.WriteString("</g>\n")
	}
}

// writeLabel puts a field's name in the hole its fence left for it. gapFrom is
// the start of that hole, so the two cannot drift apart.
func writeLabel(b *strings.Builder, label string, gapFrom, y0 int, o SVGOptions) {
	if label == "" {
		return
	}
	b.WriteString(`<text x="`)
	b.WriteString(strconv.Itoa(gapFrom + o.Scale)) // the space before the name
	b.WriteString(`" y="`)
	// Sitting on the edge, not under it: half a cap height below the line.
	b.WriteString(strconv.Itoa(y0 + o.fontSize()/3))
	b.WriteString(`">`)
	b.WriteString(escape(label))
	b.WriteString("</text>\n")
}

// cut removes the span [from, to) from the segment [a, b), which is how the
// field's name gets a hole in the fence to sit in.
func cut(a, b, from, to int) [][2]int {
	if from >= to || to <= a || from >= b {
		return [][2]int{{a, b}}
	}
	var out [][2]int
	if from > a {
		out = append(out, [2]int{a, from})
	}
	if to < b {
		out = append(out, [2]int{to, b})
	}
	return out
}

func moveH(d *strings.Builder, x, y, to int) {
	if to <= x {
		return
	}
	d.WriteByte('M')
	d.WriteString(strconv.Itoa(x))
	d.WriteByte(' ')
	d.WriteString(strconv.Itoa(y))
	d.WriteByte('H')
	d.WriteString(strconv.Itoa(to))
}

func moveV(d *strings.Builder, x, y, to int) {
	if to <= y {
		return
	}
	d.WriteByte('M')
	d.WriteString(strconv.Itoa(x))
	d.WriteByte(' ')
	d.WriteString(strconv.Itoa(y))
	d.WriteByte('V')
	d.WriteString(strconv.Itoa(to))
}

// ---- the farmer -------------------------------------------------------

func (s *Scene) farmerSpan(t *Theme) int {
	_, _, span, ok := s.FarmerSpot(t)
	if !ok {
		return 0
	}
	return span
}

// writeFarmer draws the two frames of the farmer as two groups. With animation
// on, one is shown at a time and the pair walks; with it off, only the standing
// frame is written and the file has no moving parts at all.
func (s *Scene) writeFarmer(b *strings.Builder, t *Theme, o SVGOptions) {
	x, y, span, ok := s.FarmerSpot(t)
	if !ok {
		return
	}

	frame := func(sprite Sprite, class, attrs string) {
		c := NewCanvas(o.Cols, o.Rows*2)
		c.Blit(sprite, x, y, t)
		if o.Night {
			c.Disc(x+sprite.W()+1, y+sprite.H()-2, 1, t.Colour(RoleLamp))
		}

		b.WriteString(`<g class="`)
		b.WriteString(class)
		b.WriteByte('"')
		b.WriteString(attrs)
		b.WriteString(">\n")
		writePixels(b, c, o.Scale)
		b.WriteString("</g>\n")
	}

	if !o.Animate || span <= 0 {
		frame(t.Sprites.Farmer, "farmer", "")
		return
	}

	b.WriteString("<g class=\"walk\">\n")
	frame(t.Sprites.Farmer, "step-a", "")
	// The second frame is hidden by a presentation attribute, which anything
	// that runs the keyframes will override. A renderer that ignores CSS —
	// a thumbnailer, an image library — then shows one farmer standing still
	// rather than two of them on top of each other.
	frame(t.Sprites.FarmerWalk, "step-b", ` fill-opacity="0"`)
	b.WriteString("</g>\n")
}

// writeStyle is every moving part in the file, which is a deliberate list of
// two: the farmer's stride, and which of their two frames is showing.
//
// CSS keyframes, never a script. GitHub strips <script> from an SVG, so an
// animation that depends on one is an animation that works everywhere except
// the one place this picture is for.
//
// The two frames are swapped with fill-opacity rather than opacity on purpose.
// A README embeds this file with <img>, which makes it an image document, and
// an image document is rendered without a compositor — so the properties a
// browser would normally hand to the compositor, opacity among them, are the
// ones at risk of being dropped. fill-opacity is a paint property and is
// computed wherever the pixels are.
//
// Verified here: the animation runs when the file is opened as a document in
// Chrome. Not verified here: the same file inside an <img>, because headless
// Chrome does not advance animations in an image document at all, whatever
// property they use. The approach is the one the contribution-snake action
// relies on, and phase 3 is what will prove it end to end.
func writeStyle(b *strings.Builder, o SVGOptions, span int) {
	b.WriteString("<style>\n")
	b.WriteString(`.label{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,"DejaVu Sans Mono",monospace}` + "\n")

	if o.Animate && span > 0 {
		steps := strconv.Itoa(span * o.Scale)
		b.WriteString(".walk{animation:walk 14s ease-in-out infinite alternate}\n")
		b.WriteString("@keyframes walk{from{transform:translateX(0)}to{transform:translateX(" + steps + "px)}}\n")
		b.WriteString(".step-a{animation:step-a .7s steps(1,end) infinite}\n")
		b.WriteString(".step-b{animation:step-b .7s steps(1,end) infinite}\n")
		b.WriteString("@keyframes step-a{0%{fill-opacity:1}50%{fill-opacity:0}}\n")
		b.WriteString("@keyframes step-b{0%{fill-opacity:0}50%{fill-opacity:1}}\n")
		// Somebody who has asked their machine for less movement has asked this
		// picture too.
		b.WriteString("@media(prefers-reduced-motion:reduce){")
		b.WriteString(".walk,.step-a,.step-b{animation:none}.step-b{fill-opacity:0}}\n")
	}
	b.WriteString("</style>\n")
}

// ---- small things -----------------------------------------------------

func packed(c color.RGBA) uint32 {
	return uint32(c.R)<<16 | uint32(c.G)<<8 | uint32(c.B)
}

const hexDigits = "0123456789abcdef"

func hex(c color.RGBA) string {
	out := []byte{'#', 0, 0, 0, 0, 0, 0}
	for i, v := range []uint8{c.R, c.G, c.B} {
		out[1+i*2] = hexDigits[v>>4]
		out[2+i*2] = hexDigits[v&0x0f]
	}
	return string(out)
}

func escape(s string) string {
	return strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	).Replace(s)
}
