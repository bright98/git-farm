package farm

import (
	"image/color"
	"strings"
	"testing"
)

// The picture is made of colours, so it cannot be read from the escape codes.
// This turns each role back into a letter, which makes the layout reviewable in
// a diff and in a test log. It found three real bugs in the demo that were
// invisible in a colour dump: tall plants growing through the fence, only one
// row of plants fitting in a field, and deleted-file marks being the same
// colour as the soil.
//
// Ordered, not a map: a theme can give two roles the same colour, and the first
// entry wins. Map order would make the dump change between runs.
var roleChars = []struct {
	role Role
	ch   rune
}{
	{RoleFence, '#'}, {RoleFencePost, '@'},
	{RoleLeaf, '*'}, {RoleLeafHi, '*'}, {RoleStem, '|'},
	{RoleWeed, 'w'}, {RoleWeedHi, 'w'},
	{RoleDry, 'd'}, {RoleDryStem, 'i'},
	{RoleSun, 'O'}, {RoleMoon, 'C'}, {RoleSunGlow, 'o'},
	{RoleCloud, '~'}, {RoleStar, '`'},
	{RoleHat, 'F'}, {RoleSkin, 'F'}, {RoleShirt, 'F'},
	{RolePants, 'F'}, {RoleBoots, 'F'}, {RoleLamp, 'L'},
	{RoleGrass, '.'}, {RoleGrassAlt, ','},
	{RoleSoil, ':'}, {RoleSoilAlt, ':'}, {RoleSoilBare, '_'}, {RoleSoilHole, 'x'},
	{RoleLabel, 'T'},
}

// ascii dumps the pixel buffer. The character overlay — borders and labels — is
// drawn on top, so a field's edge shows up as the glyph it really is.
func (c *Canvas) ascii(t *Theme) string {
	chars := map[color.RGBA]rune{}
	for _, rc := range roleChars {
		col := t.Colour(rc.role)
		if col.A == 0 {
			continue
		}
		if _, taken := chars[col]; !taken {
			chars[col] = rc.ch
		}
	}

	var b strings.Builder
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			if r := c.runes[(y/2)*c.W+x]; r != 0 {
				b.WriteRune(r)
				continue
			}
			px := c.At(x, y)
			switch {
			case px.A == 0:
				b.WriteByte(' ') // nothing painted: the terminal shows through
			default:
				if r, ok := chars[px]; ok {
					b.WriteRune(r)
				} else if r, ok := chars[undarken(px)]; ok {
					b.WriteRune(r)
				} else {
					b.WriteByte('+') // sky gradient and other blended pixels
				}
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func undarken(c color.RGBA) color.RGBA {
	return color.RGBA{uint8(float64(c.R) / 0.88), uint8(float64(c.G) / 0.88), uint8(float64(c.B) / 0.88), 255}
}

// sample is a farm with one of each kind of field, and one of each state of
// fence, so a dump shows every case at once.
func sample() *Scene {
	return &Scene{
		Author: "Ada",
		Farmer: 1,
		Fields: []Field{
			{
				Name: "internal/", Kind: Healthy, Fence: FenceSolid,
				Counts: Counts{Plain: 40, Tall: 4}, Weight: 44,
			},
			{
				Name: "internal/store/", Kind: Hotspot, Fence: FenceSolid,
				Counts: Counts{Plain: 6, Tall: 2, Weed: 14}, Weight: 22,
			},
			{
				Name: "migrations/", Kind: Untested, Fence: FenceBroken,
				Counts: Counts{Plain: 18}, Weight: 18,
			},
			{
				Name: "legacy/", Kind: Dead, Fence: FenceUnknown,
				Counts: Counts{Dry: 12, Hole: 5}, Weight: 17,
			},
		},
	}
}

func TestDumpFarm(t *testing.T) {
	for _, theme := range []*Theme{&Quiet, &Full} {
		for _, night := range []bool{false, true} {
			c := NewCanvas(100, 64)
			sample().Draw(c, Options{Theme: theme, Night: night, Names: true})

			when := "day"
			if night {
				when = "night"
			}
			t.Log("\n" + theme.Name + " / " + when + "\n" + c.ascii(theme))
		}
	}
}

// A quiet frame must leave most of the screen untouched, or "it sits in your
// session" is a lie. It must also still draw something.
func TestQuietIsMostlyTransparent(t *testing.T) {
	c := NewCanvas(100, 64)
	sample().Draw(c, Options{Theme: &Quiet, Names: true})

	painted := 0
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			if c.At(x, y).A != 0 {
				painted++
			}
		}
	}

	if painted == 0 {
		t.Fatal("the quiet theme painted nothing")
	}
	if got := 100 * painted / (c.W * c.H); got > 20 {
		t.Errorf("the quiet theme painted %d%% of the canvas, want at most 20%%", got)
	}
}

// The full theme covers every pixel, so its frame must contain no
// default-background code at all: one would be a hole in the picture.
func TestFullPaintsEverything(t *testing.T) {
	out := sample().Render(60, 20, Options{Theme: &Full}, TrueColor)
	if strings.Contains(out, "\x1b[49m") {
		t.Error("the full theme left a gap: found a default-background code")
	}
}

// `git farm > farm.txt` and `git farm | less` must not be full of escape codes.
// This is the failure people notice and never report.
func TestMonoWritesNoEscapes(t *testing.T) {
	out := sample().Render(80, 24, Options{Theme: &Quiet, Names: true}, Mono)
	if strings.Contains(out, "\x1b") {
		t.Error("the mono profile wrote an escape code")
	}
	if !strings.Contains(out, "▀") && !strings.Contains(out, "█") {
		t.Error("the mono profile drew no plants: shape has to survive without colour")
	}
}

// Colour is the channel you cannot rely on: the quiet palette's green, amber
// and purple sit at almost the same lightness, so in greyscale they are one
// shade. Two sprites that mean different things must therefore differ as
// shapes. This counts the pixels set in one and not the other.
func TestQuietSilhouettesDiffer(t *testing.T) {
	for _, set := range spriteSets() {
		t.Run(set.name, func(t *testing.T) { silhouettesDiffer(t, set.set) })
	}
}

// The sets whose colours cannot be relied on: the quiet palette's green, amber
// and purple sit at almost the same lightness in both of them.
func spriteSets() []struct {
	name string
	set  SpriteSet
} {
	return []struct {
		name string
		set  SpriteSet
	}{
		{"terminal", Quiet.Sprites},
		{"file", svgSprites},
	}
}

func silhouettesDiffer(t *testing.T, set SpriteSet) {
	named := []struct {
		name string
		s    Sprite
	}{
		{"plant", set.Plant}, {"big", set.Tall},
		{"weed", set.Weed}, {"dry", set.Dry},
	}

	filled := func(s Sprite, x, y int) bool {
		if y >= len(s.Art) || x >= len(s.Art[y]) {
			return false
		}
		return s.Art[y][x] != '.'
	}

	for i := 0; i < len(named); i++ {
		for j := i + 1; j < len(named); j++ {
			a, b := named[i], named[j]
			diff := 0
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					if filled(a.s, x, y) != filled(b.s, x, y) {
						diff++
					}
				}
			}
			if diff < 4 {
				t.Errorf("%s and %s differ by only %d pixels with colour removed",
					a.name, b.name, diff)
			}
		}
	}
}

// The three states of a plant are also spaced by how much ink they use, so a
// field reads as overgrown or bare before any single shape resolves.
func TestQuietInkDensity(t *testing.T) {
	ink := func(s Sprite) int {
		n := 0
		for _, row := range s.Art {
			for i := 0; i < len(row); i++ {
				if row[i] != '.' {
					n++
				}
			}
		}
		return n
	}

	for _, set := range spriteSets() {
		dry, plant, weed := ink(set.set.Dry), ink(set.set.Plant), ink(set.set.Weed)
		if !(dry < plant && plant < weed) {
			t.Errorf("%s: ink must rise dry < plant < weed, got %d, %d, %d",
				set.name, dry, plant, weed)
		}
		if weed < 2*dry {
			t.Errorf("%s: weed (%d) should be at least twice the ink of dry (%d)",
				set.name, weed, dry)
		}
	}
}

// The fence is the one claim that can be wrong in public. Its three states must
// be three visibly different things, in both themes.
func TestFenceStatesDiffer(t *testing.T) {
	for _, theme := range []*Theme{&Quiet, &Full} {
		seen := map[string]Fence{}
		for _, fence := range []Fence{FenceSolid, FenceBroken, FenceUnknown} {
			c := NewCanvas(60, 40)
			s := &Scene{Farmer: -1, Fields: []Field{
				{Name: "pkg/", Fence: fence, Counts: Counts{Plain: 6}, Weight: 6},
			}}
			s.Draw(c, Options{Theme: theme})

			// The whole picture, as letters. Everything but the edge is
			// identical between the three — same field, same seed — so any
			// difference at all is the fence.
			dump := c.ascii(theme)
			if other, clash := seen[dump]; clash {
				t.Errorf("%s theme draws fence %v exactly like fence %v", theme.Name, fence, other)
			}
			seen[dump] = fence
		}
	}
}
