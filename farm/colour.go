package farm

import (
	"image/color"
	"os"
	"strconv"
	"strings"

	"golang.org/x/term"
)

// A Profile is how much colour the terminal can be asked for.
//
// The demo assumed 24-bit colour. Real terminals do not all have it, and the
// tool must not look broken on the ones that do not. This costs almost nothing
// to support: the palette is seven colours, so the mapping is seven lookups
// done once.
type Profile int

const (
	// Mono writes no colour codes at all. This is what `git farm > farm.txt`
	// and `git farm | less` get, and it is not a degraded picture so much as a
	// one-bit one: the sprites are held apart by silhouette and ink density
	// before colour, which is exactly what makes this readable.
	Mono Profile = iota

	ANSI16    // the eight colours and their bright halves, from the user's own theme
	ANSI256   // the 216-colour cube and 24 greys
	TrueColor // ESC[38;2;r;g;b
)

func (p Profile) String() string {
	switch p {
	case TrueColor:
		return "truecolor"
	case ANSI256:
		return "256 colours"
	case ANSI16:
		return "16 colours"
	default:
		return "no colour"
	}
}

// Detect asks the environment what the terminal can do.
//
// The order matters. NO_COLOR and "this is not a terminal" come first, because
// the worst outcome here is not a dull picture — it is filling somebody's
// redirected file with escape codes.
func Detect(f *os.File) Profile {
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return Mono
	}
	if f == nil || !term.IsTerminal(int(f.Fd())) {
		return Mono
	}

	termVar := os.Getenv("TERM")
	if termVar == "" || termVar == "dumb" {
		return Mono
	}

	switch strings.ToLower(os.Getenv("COLORTERM")) {
	case "truecolor", "24bit":
		return TrueColor
	}
	switch {
	case strings.Contains(termVar, "truecolor"), strings.Contains(termVar, "direct"):
		return TrueColor
	case strings.Contains(termVar, "256"):
		return ANSI256
	default:
		return ANSI16
	}
}

// ParseProfile reads the --color flag.
func ParseProfile(name string) (Profile, bool) {
	switch strings.ToLower(name) {
	case "auto":
		return Detect(os.Stdout), true
	case "full", "truecolor", "24bit":
		return TrueColor, true
	case "256":
		return ANSI256, true
	case "16", "ansi":
		return ANSI16, true
	case "none", "off", "no":
		return Mono, true
	}
	return Mono, false
}

// Paint wraps a string in one colour, in whatever the profile can express. It
// is for the legend and the caption; the picture itself goes through Render,
// which only writes a code when the colour actually changes.
func (p Profile) Paint(s string, c color.RGBA) string {
	if p == Mono || c.A == 0 {
		return s
	}
	var b strings.Builder
	p.sgr(&b, 38, c)
	b.WriteString(s)
	b.WriteString("\x1b[0m")
	return b.String()
}

// sgr is the escape code for one colour at one layer (38 foreground, 48
// background), in whatever the profile can express.
func (p Profile) sgr(b *strings.Builder, layer int, c color.RGBA) {
	switch p {
	case TrueColor:
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(layer))
		b.WriteString(";2;")
		b.WriteString(strconv.Itoa(int(c.R)))
		b.WriteByte(';')
		b.WriteString(strconv.Itoa(int(c.G)))
		b.WriteByte(';')
		b.WriteString(strconv.Itoa(int(c.B)))
		b.WriteByte('m')

	case ANSI256:
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(layer))
		b.WriteString(";5;")
		b.WriteString(strconv.Itoa(int(nearest256(c))))
		b.WriteByte('m')

	case ANSI16:
		// The basic sixteen have their own codes rather than an index, because
		// that is what lets the user's own theme decide the actual shade —
		// which is the whole point of dropping to 16 colours rather than
		// insisting on a specific brown.
		n := nearest16(c)
		base := 30
		if layer == 48 {
			base = 40
		}
		if n >= 8 {
			base += 60 // the bright half: 90-97 and 100-107
			n -= 8
		}
		b.WriteString("\x1b[")
		b.WriteString(strconv.Itoa(base + int(n)))
		b.WriteByte('m')
	}
}

// The 216-colour cube's channel levels, and the 24 greys that sit after it.
var cubeLevels = [6]int{0, 95, 135, 175, 215, 255}

// nearest256 maps a colour to the xterm 256 palette: the cube, or the grey
// ramp, whichever is closer. Trying only the cube turns the palette's greys
// into muddy off-colours, and the field edges are grey.
func nearest256(c color.RGBA) uint8 {
	r, g, b := int(c.R), int(c.G), int(c.B)

	cube := func(v int) (idx, level int) {
		best, bestD := 0, 1<<30
		for i, lv := range cubeLevels {
			if d := abs(v - lv); d < bestD {
				best, bestD = i, d
			}
		}
		return best, cubeLevels[best]
	}

	ri, rl := cube(r)
	gi, gl := cube(g)
	bi, bl := cube(b)
	cubeIdx := uint8(16 + 36*ri + 6*gi + bi)
	cubeDist := dist(r, g, b, rl, gl, bl)

	// The grey ramp: 8, 18, 28 ... 238.
	grey := (r + g + b) / 3
	step := (grey - 8) / 10
	if step < 0 {
		step = 0
	}
	if step > 23 {
		step = 23
	}
	level := 8 + step*10
	if greyDist := dist(r, g, b, level, level, level); greyDist < cubeDist {
		return uint8(232 + step)
	}
	return cubeIdx
}

// The sixteen ANSI colours as most terminals actually render them. These are
// only used to pick the closest name; the terminal paints its own shade.
var ansi16 = [16][3]int{
	{0, 0, 0}, {205, 49, 49}, {13, 188, 121}, {229, 229, 16},
	{36, 114, 200}, {188, 63, 188}, {17, 168, 205}, {229, 229, 229},
	{102, 102, 102}, {241, 76, 76}, {35, 209, 139}, {245, 245, 67},
	{59, 142, 234}, {214, 112, 214}, {41, 184, 219}, {255, 255, 255},
}

func nearest16(c color.RGBA) uint8 {
	best, bestD := 0, 1<<30
	for i, v := range ansi16 {
		if d := dist(int(c.R), int(c.G), int(c.B), v[0], v[1], v[2]); d < bestD {
			best, bestD = i, d
		}
	}
	return uint8(best)
}

// dist is a weighted squared distance. The weights are the usual approximation
// of how much each channel contributes to perceived brightness; plain Euclidean
// distance in RGB picks visibly wrong greens.
func dist(r1, g1, b1, r2, g2, b2 int) int {
	dr, dg, db := r1-r2, g1-g2, b1-b2
	return 2*dr*dr + 4*dg*dg + 3*db*db
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
