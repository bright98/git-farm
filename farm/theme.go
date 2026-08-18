package farm

import "image/color"

func rgb(v uint32) color.RGBA {
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
}

// A Role is a thing in the picture, not a colour. Sprites and the scene ask for
// roles; the Theme decides what colour each one is. That is what lets a second
// theme exist without a second copy of the drawing code — and what lets the
// quiet theme delete the sky without a single if: a role with no colour is
// never drawn.
type Role uint8

const (
	RoleNone Role = iota

	RoleLeaf
	RoleLeafHi
	RoleStem
	RoleWeed
	RoleWeedHi
	RoleDry
	RoleDryStem

	RoleHat
	RoleSkin
	RoleShirt
	RolePants
	RoleBoots
	RoleLamp

	RoleSky
	RoleSkyLow
	RoleSun
	RoleSunGlow
	RoleMoon
	RoleStar
	RoleCloud

	RoleGrass
	RoleGrassAlt
	RoleSoil
	RoleSoilAlt
	RoleSoilBare
	RoleSoilHole

	RoleFence
	RoleFencePost
	RoleLabel // the directory name written along the top of a field

	roleCount
)

// A SpriteSet is the art a theme draws with. The quiet theme uses smaller,
// sparser sprites; the shape of the scene is the same either way.
type SpriteSet struct {
	Plant, Tall, Weed, Dry Sprite
	Farmer, FarmerWalk     Sprite
}

// A Border is how a theme draws a field edge.
type Border int

const (
	BorderFence Border = iota // wooden rails with a post every sixth pixel
	BorderGlyph               // box-drawing characters: thinner than a pixel
)

// A Fence is what the edge says about tests. There are three of these and not
// two on purpose: the third is the difference between "this directory has no
// tests" and "nothing here can be checked", and only one of those is safe to
// print on somebody else's README.
type Fence int

const (
	FenceSolid   Fence = iota // test files, or inline tests, were found
	FenceBroken               // the rules apply here and found nothing
	FenceUnknown              // no rule applies: the corners are marked, and no claim is made
)

type Theme struct {
	Name string

	// Sky paints the gradient, the clouds and the stars behind everything.
	// Off means those pixels are never touched.
	Sky bool

	// Sun draws the sun by day and the moon by night. It is separate from Sky
	// because the quiet theme wants the one without the other: the time of day
	// is worth a mark, the backdrop is not.
	Sun bool

	// Ground paints grass and soil. Off leaves the terminal's own background.
	Ground bool

	// Border is how the field edge is drawn.
	Border Border

	// Outline draws the sun and moon as a ring and a crescent rather than
	// filled discs. A filled shape would punch a hole in a transparent picture.
	Outline bool

	// StepX and StepY are how far apart plants stand, in pixels. More space is
	// not decoration: it is what makes a sparse field read as sparse.
	StepX, StepY int

	Sprites SpriteSet
	colours [roleCount]color.RGBA
}

func (t *Theme) Colour(r Role) color.RGBA { return t.colours[r] }

// Full is a whole little world, sky and all. It is the theme for the README
// image, where there is no session for the picture to sit in.
var Full = Theme{
	Name:    "full",
	Sky:     true,
	Sun:     true,
	Ground:  true,
	Border:  BorderFence,
	StepX:   7,
	StepY:   7,
	Sprites: fullSprites,
	colours: [roleCount]color.RGBA{
		RoleLeaf:   rgb(0x4f9b2e),
		RoleLeafHi: rgb(0x7ecb4f),
		RoleStem:   rgb(0x3a6f22),
		RoleWeed:   rgb(0x9c8f36),
		RoleWeedHi: rgb(0xbfae45),
		RoleDry:    rgb(0x9a9276),

		RoleDryStem: rgb(0x7c745c),
		RoleHat:     rgb(0xd94f3d),
		RoleSkin:    rgb(0xf0c090),
		RoleShirt:   rgb(0x3f6fd9),
		RolePants:   rgb(0x33406b),
		RoleBoots:   rgb(0x2a2118),
		RoleLamp:    rgb(0xffd050),

		RoleSky:     rgb(0x7ec8f0),
		RoleSkyLow:  rgb(0xb8e2f7),
		RoleSun:     rgb(0xffd94a),
		RoleSunGlow: rgb(0xffec9e),
		RoleMoon:    rgb(0xe8ecf5),
		RoleStar:    rgb(0xfdfbe8),
		RoleCloud:   rgb(0xf2f7fb),

		RoleGrass:    rgb(0x5a9c34),
		RoleGrassAlt: rgb(0x4f8a2c),
		RoleSoil:     rgb(0x6b4a2f),
		RoleSoilAlt:  rgb(0x5b3e27),
		RoleSoilBare: rgb(0x836a55),
		RoleSoilHole: rgb(0x4a3a2c),

		RoleFence:     rgb(0xb0824f),
		RoleFencePost: rgb(0xd0a370),
		RoleLabel:     rgb(0xf7ecd8),
	},
}

// Quiet keeps every element and throws away everything else: no sky, no sun, no
// grass, no soil. Those pixels are left untouched, so the terminal's own
// background shows through and the farm sits in the session instead of covering
// it. This is the one to default to in a terminal.
//
// The palette is the arcade's — one accent, one green, one amber, one grey — so
// git-farm and ssh-games look like the same hand made them.
var Quiet = quiet()

func quiet() Theme {
	accent := rgb(0xd7bcff) // purple
	text := rgb(0xf4f4f4)
	label := rgb(0xa8b0ac)  // field names: brighter than the edge they sit on
	subtle := rgb(0x79817d) // field edges: dimmer than what they frame
	faint := rgb(0x6f7873)  // dead and deleted
	green := rgb(0x8fd39a)  // a live file
	amber := rgb(0xf2d98a)  // churn
	warm := rgb(0xf7e6b0)   // the sun
	cool := rgb(0xdfe7f5)   // the moon

	t := Theme{
		Name:    "quiet",
		Sky:     false, // no gradient, no clouds, no stars
		Sun:     true,  // but keep the sun and the moon
		Ground:  false,
		Border:  BorderGlyph,
		Outline: true,
		StepX:   7,
		StepY:   5,
		Sprites: quietSprites,
	}
	set := func(r Role, c color.RGBA) { t.colours[r] = c }

	set(RoleLeaf, green)
	set(RoleLeafHi, green)
	set(RoleStem, green)
	set(RoleWeed, amber)
	set(RoleWeedHi, amber)
	set(RoleDry, faint)
	set(RoleDryStem, faint)

	// A big file is the one thing the accent is spent on.
	set(RoleHat, accent)
	set(RoleSkin, text)
	set(RoleShirt, accent)
	set(RolePants, subtle)
	set(RoleBoots, subtle)
	set(RoleLamp, warm)

	set(RoleSun, warm)
	set(RoleMoon, cool)

	// A deleted file is a single faint mark, not a hole in painted soil.
	set(RoleSoilHole, faint)

	set(RoleFence, subtle)
	set(RoleFencePost, accent)
	set(RoleLabel, label)

	// Everything not set above stays zero, which means transparent: the sky
	// gradient, the sun's glow, the clouds, the stars, the grass and the soil
	// are simply never drawn.
	return t
}

// QuietLight is the quiet theme for a page with a white background.
//
// The TUI does not need this — lipgloss can ask the terminal which it is and
// pick per session — but a file on disk cannot ask anybody, so the SVG ships
// one of these and one of Quiet, and the README picks with prefers-color-scheme.
//
// It is not the same palette darkened. The greens and ambers have to gain
// weight to hold their own against white, and the greys have to lose it:
// on a dark page dead code recedes by being dim, and on a light one it recedes
// by being pale.
var QuietLight = quietLight()

func quietLight() Theme {
	t := quiet()
	t.Name = "quiet-light"

	set := func(r Role, c color.RGBA) { t.colours[r] = c }

	accent := rgb(0x7a52c9) // purple, with enough weight for white
	green := rgb(0x3f8b52)
	amber := rgb(0x9a7318)
	faint := rgb(0xb4bcb7) // dead and deleted: pale, so it recedes
	subtle := rgb(0x9aa29e)
	label := rgb(0x6b736f)

	set(RoleLeaf, green)
	set(RoleLeafHi, green)
	set(RoleStem, green)
	set(RoleWeed, amber)
	set(RoleWeedHi, amber)
	set(RoleDry, faint)
	set(RoleDryStem, faint)

	set(RoleHat, accent)
	set(RoleSkin, rgb(0x3a3f3c))
	set(RoleShirt, accent)
	set(RolePants, subtle)
	set(RoleBoots, subtle)
	set(RoleLamp, rgb(0xd9a520))

	set(RoleSun, rgb(0xd9a520))
	set(RoleMoon, rgb(0x7f8aa3))

	set(RoleSoilHole, faint)
	set(RoleFence, subtle)
	set(RoleFencePost, accent)
	set(RoleLabel, label)

	return t
}

// Themes is every theme by name, for the --theme flag.
var Themes = []*Theme{&Quiet, &QuietLight, &Full}

// ThemeNamed returns the theme with this name, or nil.
func ThemeNamed(name string) *Theme {
	for _, t := range Themes {
		if t.Name == name {
			return t
		}
	}
	return nil
}
