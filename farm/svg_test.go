package farm

import (
	"encoding/xml"
	"strings"
	"testing"
)

func draw(t *testing.T, o SVGOptions) string {
	t.Helper()
	var b strings.Builder
	if err := sample().WriteSVG(&b, o); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Whatever else is true of the file, it has to be a well-formed XML document.
func TestSVGIsWellFormed(t *testing.T) {
	out := draw(t, SVGOptions{Theme: &Quiet, Names: true, Animate: true})

	d := xml.NewDecoder(strings.NewReader(out))
	depth := 0
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		switch tok.(type) {
		case xml.StartElement:
			depth++
		case xml.EndElement:
			depth--
		}
	}
	if depth != 0 {
		t.Errorf("the document does not close cleanly: depth %d at the end", depth)
	}
	if !strings.HasPrefix(out, "<svg xmlns=") {
		t.Error("the file does not start with an svg element")
	}
}

// GitHub strips <script> from an SVG, so an animation that needs one is an
// animation that works everywhere except the place this picture is for.
func TestSVGHasNoScript(t *testing.T) {
	out := draw(t, SVGOptions{Theme: &Quiet, Names: true, Animate: true})

	for _, banned := range []string{"<script", "javascript:", "onload=", "<foreignObject"} {
		if strings.Contains(strings.ToLower(out), banned) {
			t.Errorf("the file contains %q, which GitHub will strip or refuse", banned)
		}
	}
	if !strings.Contains(out, "@keyframes walk") {
		t.Error("nothing animates the farmer")
	}
}

// One path per colour, not one rect per pixel. A 120x72 farm is 8,640 pixels;
// if this ever goes back to a rect each, the file is a megabyte.
func TestSVGIsMergedIntoPaths(t *testing.T) {
	out := draw(t, SVGOptions{Theme: &Full, Names: true, Animate: true})

	// The full theme's sky is a gradient, so it costs a colour a row; the quiet
	// theme has seven colours and nothing behind them.
	if paths := strings.Count(out, "<path"); paths > 80 {
		t.Errorf("%d paths: the runs are not being merged", paths)
	}
	quiet := draw(t, SVGOptions{Theme: &Quiet, Names: true, Animate: true})
	if paths := strings.Count(quiet, "<path"); paths > 20 {
		t.Errorf("%d paths for a seven-colour palette", paths)
	}
	if strings.Contains(out, "<rect") {
		t.Error("a rect got in: the pixels should be merged into paths")
	}

	// And the merging has to actually merge: a farm this size cannot be drawn
	// with only single-pixel rectangles.
	if !strings.Contains(out, "v12") && !strings.Contains(out, "v18") {
		t.Error("no rectangle is taller than one pixel, so the vertical merge did nothing")
	}
}

// The same repository at the same commit has to give the same bytes, or the
// Action pushes a new picture on every run for a repo that has not changed.
func TestSVGIsDeterministic(t *testing.T) {
	o := SVGOptions{Theme: &Quiet, Names: true, Animate: true}
	if a, b := draw(t, o), draw(t, o); a != b {
		t.Error("two runs produced different bytes")
	}
}

// A file cannot ask the reader whether their page is light or dark, so it ships
// as two.
func TestSVGPalettesDiffer(t *testing.T) {
	dark := draw(t, SVGOptions{Theme: &Quiet, Names: true})
	light := draw(t, SVGOptions{Theme: &QuietLight, Names: true})

	if dark == light {
		t.Fatal("the light and dark files are identical")
	}
	if !strings.Contains(dark, hex(Quiet.Colour(RoleLeaf))) {
		t.Error("the dark file does not use the dark palette's green")
	}
	if !strings.Contains(light, hex(QuietLight.Colour(RoleLeaf))) {
		t.Error("the light file does not use the light palette's green")
	}

	// Neither one paints a background: the page shows through, which is what
	// lets one picture sit on a README of either colour.
	for name, out := range map[string]string{"dark": dark, "light": light} {
		if strings.Contains(out, `width="720" height="432" fill=`) {
			t.Errorf("the %s file paints a background rectangle", name)
		}
	}
}

// The name is written into a hole in its own fence. If the two disagree about
// where that hole is, the name has a line through it.
func TestSVGNameSitsInTheGapInTheFence(t *testing.T) {
	out := draw(t, SVGOptions{Theme: &Quiet, Names: true})

	if !strings.Contains(out, "<text") {
		t.Fatal("no names were written")
	}
	if !strings.Contains(out, "internal/store/") {
		t.Error("a field name is missing from the file")
	}

	// A field with a name has a broken top edge: two moves along the same row,
	// which is what the gap looks like in path data.
	fence := between(out, `<path class="fence`, `"/>`)
	if strings.Count(fence, "M") < 4 {
		t.Errorf("the fence is one unbroken run, so nothing made room for the name: %q", fence)
	}
}

// Real text in a picture of pixels means real text rules: a directory called
// a&b/ must not end the attribute it is written in.
func TestSVGEscapesNames(t *testing.T) {
	s := &Scene{Farmer: -1, Fields: []Field{
		{Name: `a&b<c>/`, Fence: FenceSolid, Counts: Counts{Plain: 8}, Weight: 8},
	}}

	var b strings.Builder
	if err := s.WriteSVG(&b, SVGOptions{Theme: &Quiet, Names: true, Title: `x"&y`}); err != nil {
		t.Fatal(err)
	}
	out := b.String()

	if strings.Contains(out, "a&b") || strings.Contains(out, "<c>") {
		t.Error("a name went in unescaped")
	}
	if !strings.Contains(out, "a&amp;b&lt;c&gt;/") {
		t.Error("the escaped name is missing")
	}
	if err := xml.NewDecoder(strings.NewReader(out)).Decode(new(any)); err != nil &&
		!strings.Contains(err.Error(), "unsupported type") {
		t.Errorf("the document no longer parses: %v", err)
	}
}

// Without animation there is nothing moving in the file at all — no keyframes,
// and only one farmer rather than two frames of one.
func TestSVGWithoutAnimation(t *testing.T) {
	out := draw(t, SVGOptions{Theme: &Quiet, Names: true, Animate: false})

	if strings.Contains(out, "@keyframes") {
		t.Error("keyframes in a file that was not asked to animate")
	}
	if n := strings.Count(out, `<g class="`); n > 2 {
		t.Errorf("%d groups: a still picture needs one farmer, not two frames", n)
	}
}

// A renderer that ignores CSS — a thumbnailer, an image library — must not show
// both frames of the farmer on top of each other.
func TestSVGSecondFrameIsHiddenByDefault(t *testing.T) {
	out := draw(t, SVGOptions{Theme: &Quiet, Names: true, Animate: true})

	if !strings.Contains(out, `class="step-b" fill-opacity="0"`) {
		t.Error("the second frame is not hidden for renderers that do not run CSS")
	}
	// opacity is the property a browser hands to the compositor, and an <img>
	// is rendered without one. Everything that has to move uses a paint
	// property or a transform instead.
	if strings.Contains(out, "opacity:1}") && !strings.Contains(out, "fill-opacity:1}") {
		t.Error("the frame swap animates plain opacity")
	}
	if !strings.Contains(out, "prefers-reduced-motion") {
		t.Error("somebody who asked for less movement still gets a walking farmer")
	}
}

// The wooden fence is pixel art and stays in the pixels; the box-drawing one is
// a workaround for a terminal, and SVG replaces it with a real stroke.
func TestSVGFenceFollowsTheTheme(t *testing.T) {
	if out := draw(t, SVGOptions{Theme: &Quiet, Names: true}); !strings.Contains(out, `class="fence`) {
		t.Error("the quiet theme should get a vector fence")
	}
	if out := draw(t, SVGOptions{Theme: &Full, Names: true}); strings.Contains(out, `class="fence`) {
		t.Error("the full theme's wooden fence should stay in the pixels")
	}
}

func between(s, from, to string) string {
	i := strings.Index(s, from)
	if i < 0 {
		return ""
	}
	rest := s[i+len(from):]
	j := strings.Index(rest, to)
	if j < 0 {
		return rest
	}
	return rest[:j]
}
