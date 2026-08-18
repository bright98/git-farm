package farm

import (
	"image/color"
	"strings"
	"testing"
)

// The palette is seven colours, so a wrong mapping is seven wrong colours for
// the whole session. These are the ones that matter: the greens and ambers that
// carry meaning, and the greys that would come out as muddy off-colours if the
// grey ramp were skipped.
func TestNearest256(t *testing.T) {
	cases := []struct {
		name string
		in   color.RGBA
		want uint8
	}{
		{"black", rgb(0x000000), 16},
		{"white", rgb(0xffffff), 231},
		{"pure red", rgb(0xff0000), 196},
		{"a mid grey lands in the grey ramp", rgb(0x808080), 244},
		{"a dark grey too", rgb(0x6f7873), 242},
	}
	for _, c := range cases {
		if got := nearest256(c.in); got != c.want {
			t.Errorf("%s: nearest256(%v) = %d, want %d", c.name, c.in, got, c.want)
		}
	}

	// Two palette colours that mean different things must not collapse into one
	// index, or a 256-colour terminal cannot tell churn from a live file.
	green, amber := Quiet.Colour(RoleLeaf), Quiet.Colour(RoleWeed)
	if nearest256(green) == nearest256(amber) {
		t.Error("green and amber map to the same 256-colour index")
	}
}

func TestNearest16(t *testing.T) {
	if got := nearest16(rgb(0x000000)); got != 0 {
		t.Errorf("black mapped to %d", got)
	}
	if got := nearest16(rgb(0xffffff)); got != 15 {
		t.Errorf("white mapped to %d", got)
	}

	// The quiet palette's green must land on a green, not on a grey.
	if got := nearest16(Quiet.Colour(RoleLeaf)); got != 2 && got != 10 {
		t.Errorf("the live-file green mapped to ANSI %d, want a green", got)
	}
}

// Each profile writes the kind of code that terminal understands, and mono
// writes none at all.
func TestProfileEscapes(t *testing.T) {
	col := rgb(0x8fd39a)
	cases := []struct {
		p    Profile
		want string
	}{
		{TrueColor, "\x1b[38;2;143;211;154m"},
		{ANSI256, "\x1b[38;5;"},
		{Mono, ""},
	}

	for _, c := range cases {
		var b strings.Builder
		c.p.sgr(&b, 38, col)
		if !strings.HasPrefix(b.String(), c.want) {
			t.Errorf("%v wrote %q, want it to start with %q", c.p, b.String(), c.want)
		}
	}

	// Sixteen colours get a plain SGR code — 30-37, or 90-97 for the bright
	// half — and not a palette index, so the user's own theme picks the shade.
	var b strings.Builder
	ANSI16.sgr(&b, 38, col)
	switch got := b.String(); got {
	case "\x1b[32m", "\x1b[92m":
	default:
		t.Errorf("16 colours wrote %q, want a green foreground code", got)
	}

	if got := Mono.Paint("x", col); got != "x" {
		t.Errorf("mono painted %q", got)
	}
}

// NO_COLOR is not a preference to weigh up against the others. It wins.
func TestDetectRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM", "xterm-256color")

	if got := Detect(nil); got != Mono {
		t.Errorf("NO_COLOR was set and Detect returned %v", got)
	}
}

// Nothing that is not a terminal gets colour, whatever TERM says. This is the
// one people never report: `git farm > farm.txt` full of escape codes.
func TestDetectRefusesNonTerminals(t *testing.T) {
	t.Setenv("COLORTERM", "truecolor")
	t.Setenv("TERM", "xterm-256color")

	if got := Detect(nil); got != Mono {
		t.Errorf("stdout is not a terminal and Detect returned %v", got)
	}
}

func TestParseProfile(t *testing.T) {
	cases := map[string]Profile{
		"full": TrueColor, "truecolor": TrueColor, "24bit": TrueColor,
		"256": ANSI256, "16": ANSI16, "none": Mono, "off": Mono,
	}
	for name, want := range cases {
		got, ok := ParseProfile(name)
		if !ok || got != want {
			t.Errorf("ParseProfile(%q) = %v, %v; want %v", name, got, ok, want)
		}
	}
	if _, ok := ParseProfile("chartreuse"); ok {
		t.Error("an unknown colour setting was accepted")
	}
}
