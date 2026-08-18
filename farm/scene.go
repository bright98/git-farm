package farm

import (
	"image/color"
	"math/rand"
	"strings"
)

// Kind is what a field looks like from above.
type Kind int

const (
	Healthy Kind = iota
	Hotspot
	Untested
	Dead
)

// A Field is one directory, with everything the picture needs to draw it and
// nothing else.
type Field struct {
	Name   string  // the directory path, as it will be written on the field
	Kind   Kind    //
	Fence  Fence   // what the edge says about tests
	Counts Counts  // what stands in the field
	Weight float64 // how much of the farm this field gets: its file count

	rect Rect // filled in by Layout
}

// A Scene is a whole farm: the fields, and who is standing in one of them.
type Scene struct {
	Fields []Field
	Farmer int    // the index of the field the newest commit touched, or -1
	Author string // who made that commit

	// all is every directory, before Fit decided how many the window has room
	// for. Fit works from this each time, so laying the same scene out twice
	// gives the same farm rather than gathering it away one field at a time.
	all []Field
}

// Options are the choices that do not come from the repository.
type Options struct {
	Theme  *Theme
	Night  bool
	Names  bool // draw the directory name on each field
	Walk   int  // animation frame for the farmer: 0 or 1
	Offset int  // how far the farmer has walked, in pixels
}

func (o Options) theme() *Theme {
	if o.Theme == nil {
		return &Quiet
	}
	return o.Theme
}

// The smallest window worth drawing in. Below this the farm is not a smaller
// picture, it is an unreadable one, so the tool says so instead.
const (
	MinCols = 60
	MinRows = 18
)

// A field smaller than this cannot hold a border, a row of plants and a name,
// so the layout would rather draw fewer fields than draw unreadable ones.
const (
	minFieldW = 16
	minFieldH = 12

	// More fields than this is a mosaic rather than a farm. The rest are
	// gathered into one field named other/.
	maxFields = 10
)

// Layout decides where the sky ends and where the fields go.
//
// It is computed once, from the state at HEAD, and never per frame.
func Layout(w, h int) (skyH int, area Rect) {
	// The band has to hold the sun, rays and all, or the bottom ray lands in
	// the top field and reads as a stray pixel rather than as sunshine.
	skyH = h * 18 / 100
	if skyH < 12 {
		skyH = 12
	}
	if skyH > 14 {
		skyH = 14
	}

	const margin = 2
	top := skyH + 2
	top += top % 2 // fields start on a cell boundary

	return skyH, Rect{
		X: margin,
		Y: top,
		W: w - 2*margin,
		H: h - top - 2,
	}
}

// MaxFields is how many fields an area of this size can hold at a readable
// size. It is what decides how many directories get their own field and how
// many are gathered into other/.
func MaxFields(area Rect) int {
	if area.W < minFieldW || area.H < minFieldH {
		return 1
	}
	n := area.Area() / (minFieldW * minFieldH)
	if n > maxFields {
		n = maxFields
	}
	if n < 1 {
		n = 1
	}
	return n
}

// Fit chooses how many fields the farm has room for and lays them out.
//
// How many fit is an estimate, so it is checked rather than trusted: if the
// treemap comes back with a field too small to draw, one more directory is
// gathered into other/ and the whole thing is laid out again. Fewer and bigger,
// until everything that is left is legible — and nothing silently missing.
func (s *Scene) Fit(area Rect) {
	if s.all == nil {
		s.all = s.Fields
	}

	// The farmer is remembered by name, because gathering renumbers the fields
	// underneath them.
	farmer := ""
	if s.Farmer >= 0 && s.Farmer < len(s.all) {
		farmer = s.all[s.Farmer].Name
	}

	for keep := MaxFields(area); keep >= 1; keep-- {
		s.Fields = gather(s.all, keep)
		s.Farmer = fieldNamed(s.Fields, farmer)
		if s.Place(area) || keep == 1 {
			return
		}
	}
}

// fieldNamed finds a field by name, falling back to the one that swallowed it.
func fieldNamed(fields []Field, name string) int {
	if name == "" {
		return -1
	}
	for i, f := range fields {
		if f.Name == name {
			return i
		}
	}
	for i, f := range fields {
		if f.Name == otherField {
			return i
		}
	}
	return -1
}

// Place runs the treemap over the fields' weights and records where each one
// goes. It reports whether every field came out big enough to draw.
//
// A field that does not fit is left unplaced rather than drawn as a sliver.
// Nothing is removed from the scene, because placing has to be repeatable: the
// caller answers a false by gathering more directories into other/ and asking
// again, and a mutated field list would give a different farm the second time.
func (s *Scene) Place(area Rect) bool {
	weights := make([]float64, len(s.Fields))
	for i, f := range s.Fields {
		weights[i] = f.Weight
	}

	tiles := Squarify(area, weights)

	fits := true
	for i := range s.Fields {
		r := Inset(tiles[i])
		if r.W < minFieldW || r.H < minFieldH {
			s.Fields[i].rect = Rect{}
			fits = false
			continue
		}
		s.Fields[i].rect = r
	}
	return fits
}

// Drawn is how many fields are actually going to appear.
func (s *Scene) Drawn() int {
	n := 0
	for _, f := range s.Fields {
		if f.rect.W > 0 {
			n++
		}
	}
	return n
}

// Render draws the farm at a terminal size and returns the string to print.
// cols and rows are terminal cells; each row carries two pixels.
func (s *Scene) Render(cols, rows int, o Options, p Profile) string {
	c := NewCanvas(cols, rows*2)
	s.Draw(c, o)
	return c.Render(p)
}

// Draw paints the whole scene. Everything is derived from the canvas size, so
// the picture survives a resized window.
func (s *Scene) Draw(c *Canvas, o Options) {
	t := o.theme()
	rng := rand.New(rand.NewSource(7)) // a fixed seed: the sky is the same every frame

	skyH, area := Layout(c.W, c.H)
	s.Fit(area)

	if t.Sky {
		drawSky(c, t, skyH, o.Night, rng)
	}
	if t.Sun {
		drawSunMoon(c, t, skyH, o.Night)
	}
	if t.Ground {
		drawGround(c, t, skyH)
	}

	for i, f := range s.Fields {
		if f.rect.W == 0 {
			continue
		}
		// The farmer's field keeps a strip of clear soil at the bottom to stand
		// on. Drawing the farmer over the plants instead leaves a person with a
		// plant growing through their head.
		reserve := 0
		if i == s.Farmer {
			reserve = farmerRoom(t, f.rect)
		}
		drawField(c, t, f, o, reserve)
	}

	if o.Night && t.Ground {
		dim(c, t, 0, skyH, c.W, c.H-skyH, 0.5)
	}

	s.drawFarmer(c, t, o)
}

// drawFarmer puts the author of the newest commit in the field that commit
// touched. It is the one part of the picture about a person rather than a file.
func (s *Scene) drawFarmer(c *Canvas, t *Theme, o Options) {
	if s.Farmer < 0 || s.Farmer >= len(s.Fields) {
		return
	}
	r := s.Fields[s.Farmer].rect
	if farmerRoom(t, r) == 0 {
		return // the field is too small to stand in without trampling it
	}

	sprite := t.Sprites.Farmer
	if o.Walk%2 == 1 {
		sprite = t.Sprites.FarmerWalk
	}

	span := r.W - sprite.W() - 6
	px := r.X + 3
	if span > 0 && o.Offset > 0 {
		px += o.Offset % span
	}

	c.Blit(sprite, px, r.Y+r.H-sprite.H()-2, t)
	if o.Night {
		c.Disc(px+sprite.W()+1, r.Y+r.H-5, 1, t.Colour(RoleLamp))
	}
}

// farmerRoom is how many pixels at the bottom of a field are kept clear for the
// farmer to stand in, or zero if the field is too small to hold one. Both the
// planting and the farmer ask this, so they cannot disagree.
func farmerRoom(t *Theme, r Rect) int {
	sprite := t.Sprites.Farmer
	if r.W < sprite.W()+8 || r.H < sprite.H()+minFieldH {
		return 0
	}
	return sprite.H() + 1
}

func drawSky(c *Canvas, t *Theme, skyH int, night bool, rng *rand.Rand) {
	hi, lo := t.Colour(RoleSky), t.Colour(RoleSkyLow)
	if night {
		hi, lo = skyNight, skyNightLow
	}
	for y := 0; y < skyH; y++ {
		c.Rect(0, y, c.W, 1, lerp(hi, lo, float64(y)/float64(skyH)))
	}

	if night {
		for i := 0; i < c.W/6; i++ {
			c.Set(rng.Intn(c.W), rng.Intn(skyH-2), t.Colour(RoleStar))
		}
		return
	}

	drawCloud(c, t, c.W/6, skyH/3)
	drawCloud(c, t, c.W/2, skyH/2)
}

// drawSunMoon says what time it is, in one mark.
//
// A theme that paints a sky gets a filled disc, because it has a backdrop to
// sit on and a moon can be carved out of it. A transparent theme gets an
// outline instead: filling the shape would punch a solid hole in a picture
// whose whole point is that it does not cover the terminal.
func drawSunMoon(c *Canvas, t *Theme, skyH int, night bool) {
	// Everything is measured from the centre of the band, so the whole sun —
	// rays included — stays inside the sky. A ray that lands in the top field
	// reads as a stray pixel, not as sunshine.
	cx, cy := c.W-14, skyH/2
	ray := minInt(cy-1, 6)
	rad := minInt(ray-2, 4)
	if rad < 2 {
		return // no room for a sun that would still look like one
	}

	if night {
		col := t.Colour(RoleMoon)
		if t.Outline {
			// The cut sits level with the centre, not above it: an offset in y
			// makes the two horns different lengths, which at this size reads
			// as a mistake rather than as a phase.
			c.Crescent(cx, cy, rad, rad-1, 0, col)
			return
		}
		c.Disc(cx, cy, rad, col)
		c.Disc(cx+2, cy-1, rad-1, lerp(skyNight, skyNightLow, 0.4))
		return
	}

	col := t.Colour(RoleSun)
	if t.Outline {
		c.Ring(cx, cy, rad, col)
		// Four short rays. A bare ring reads as a hole; the rays make it a sun.
		for _, d := range [][2]int{{0, -1}, {0, 1}, {-1, 0}, {1, 0}} {
			c.Set(cx+d[0]*ray, cy+d[1]*ray, col)
		}
		return
	}
	c.Disc(cx, cy, rad+2, t.Colour(RoleSunGlow))
	c.Disc(cx, cy, rad, col)
}

// The night sky is not a theme role: only a theme that paints a sky has one.
var (
	skyNight    = rgb(0x1b2340)
	skyNightLow = rgb(0x33406b)
)

func drawCloud(c *Canvas, t *Theme, x, y int) {
	col := t.Colour(RoleCloud)
	c.Disc(x, y, 2, col)
	c.Disc(x+3, y-1, 3, col)
	c.Disc(x+7, y, 2, col)
}

func drawGround(c *Canvas, t *Theme, skyH int) {
	for y := skyH; y < c.H; y++ {
		col := t.Colour(RoleGrass)
		if y%3 == 0 {
			col = t.Colour(RoleGrassAlt)
		}
		c.Rect(0, y, c.W, 1, col)
	}
}

func drawField(c *Canvas, t *Theme, f Field, o Options, reserve int) {
	r := f.rect

	if t.Ground {
		// Tilled soil, in rows, so an empty field still looks like a field.
		for dy := 1; dy < r.H-1; dy++ {
			col := t.Colour(RoleSoil)
			if f.Kind == Dead {
				col = t.Colour(RoleSoilBare)
			}
			if dy%4 < 2 {
				col = darken(col, 0.88)
			}
			c.Rect(r.X+1, r.Y+dy, r.W-2, 1, col)
		}
	}

	plantField(c, t, f, r.X+2, r.Y+2, r.W-4, r.H-4-reserve)
	drawFence(c, t, r, f.Fence)
	if o.Names {
		drawLabel(c, t, r, f.Name)
	}
}

// drawFence draws the field edge, and with it the one claim on the farm that
// can be wrong in public.
//
// Three states, not two. Solid means test files were found; dashed means the
// rules apply here and found none; corners-only means no rule applies and no
// claim is being made — a Rust file with its tests inline, a directory of SQL
// migrations, a language nobody has checked the convention for.
func drawFence(c *Canvas, t *Theme, r Rect, fence Fence) {
	line, post := t.Colour(RoleFence), t.Colour(RoleFencePost)

	if t.Border == BorderGlyph {
		drawGlyphBorder(c, r, line, post, fence)
		return
	}

	// The pixel fence: rails with a post every sixth pixel. Broken drops every
	// other run of rail. Unknown keeps only the corners and a stub of rail at
	// each one — crop marks rather than a fence, which says "this is how far
	// the field goes" without saying anything about its tests.
	put := func(i, n, x, y int) {
		if fence == FenceUnknown && i > 3 && i < n-4 {
			return
		}
		if i%6 == 0 {
			c.Set(x, y, post)
			return
		}
		if fence == FenceBroken && (i/3)%2 == 1 {
			return
		}
		c.Set(x, y, line)
	}

	for i := 0; i < r.W; i++ {
		put(i, r.W, r.X+i, r.Y)
		put(i, r.W, r.X+i, r.Y+r.H-1)
	}
	for i := 0; i < r.H; i++ {
		put(i, r.H, r.X, r.Y+i)
		put(i, r.H, r.X+r.W-1, r.Y+i)
	}
}

// drawGlyphBorder frames a field with box-drawing characters instead of pixels.
//
// This is the thinnest a terminal goes. A pixel is half a cell tall but a full
// cell wide, so a one-pixel edge is still a character wide; "│" is about an
// eighth of that. It also gives back the distinction the pixel version had to
// fake: box-drawing has real dashed variants, so tested and untested can be
// solid and dashed at the same weight, rather than one of them being chewed up.
//
// A cell replaces both of its pixels, so this must run after the field is drawn
// and the border row must be otherwise empty — which it is: plants start two
// pixels inside.
func drawGlyphBorder(c *Canvas, r Rect, line, post color.RGBA, fence Fence) {
	top, bottom := r.Y/2, (r.Y+r.H-1)/2
	if top == bottom {
		return // the field is less than one cell tall
	}
	right := r.X + r.W - 1

	// The corners are always drawn. They are what says "this is one field",
	// which stays true whatever is or is not known about its tests.
	c.SetRune(r.X, top, '┌', post)
	c.SetRune(right, top, '┐', post)
	c.SetRune(r.X, bottom, '└', post)
	c.SetRune(right, bottom, '┘', post)

	h, v := '─', '│'
	if fence == FenceBroken {
		h, v = '┄', '┆' // no test files: the same weight, dashed
	}

	// Unknown draws a stub of edge at each corner and nothing in between: crop
	// marks rather than a fence. The field is still one legible rectangle, and
	// no claim about its tests has been made.
	near := func(i, lo, hi int) bool {
		return fence != FenceUnknown || i-lo < 3 || hi-i < 3
	}

	for x := r.X + 1; x < right; x++ {
		if !near(x, r.X, right) {
			continue
		}
		c.SetRune(x, top, h, line)
		c.SetRune(x, bottom, h, line)
	}
	for y := top + 1; y < bottom; y++ {
		if !near(y, top, bottom) {
			continue
		}
		c.SetRune(r.X, y, v, line)
		c.SetRune(right, y, v, line)
	}
}

// drawLabel writes the directory's name into the top edge of its field.
//
// This is the one thing the pixel grid cannot do and the character overlay can:
// real text, at real text resolution, inside a picture made of half blocks. It
// is also what turns the farm from a pretty texture into a picture of a
// particular repository — without the names, nobody recognises their own code.
func drawLabel(c *Canvas, t *Theme, r Rect, name string) {
	// Two cells for the corners, two for the space either side of the name, and
	// two more so the name never runs into the corner it is written beside.
	room := r.W - 6
	if room < 4 {
		return
	}

	label := shorten(name, room)
	if label == "" {
		return
	}

	x := r.X + 2
	c.SetText(x, r.Y/2, " "+label+" ", t.Colour(RoleLabel))
}

// shorten fits a directory path into the space a field has, by dropping leading
// segments before it resorts to cutting the name itself. "internal/store/" is
// worth more as "store/" than as "internal…".
func shorten(name string, room int) string {
	if len(name) <= room {
		return name
	}

	trimmed := strings.TrimSuffix(name, "/")
	for {
		i := strings.Index(trimmed, "/")
		if i < 0 {
			break
		}
		trimmed = trimmed[i+1:]
		if len(trimmed)+1 <= room {
			return trimmed + "/"
		}
	}

	if room < 2 {
		return ""
	}
	return trimmed[:room-1] + "…"
}

func plantField(c *Canvas, t *Theme, f Field, x, y, w, h int) {
	// head is the room a tall plant needs above a normal one. Every plant in a
	// row stands on the same baseline, so a big file grows upwards, like a real
	// plant, instead of sinking into the soil.
	head := t.Sprites.Tall.H() - t.Sprites.Plant.H()

	stepX, stepY := t.StepX, t.StepY
	cols := w / stepX
	rows := (h - head) / stepY
	if cols < 1 || rows < 1 {
		return
	}

	items := plant(f.Name, f.Counts, cols*rows)
	for r := 0; r < rows; r++ {
		for i := 0; i < cols; i++ {
			px := x + i*stepX
			py := y + head + r*stepY

			switch items[r*cols+i] {
			case ItemTall:
				c.Blit(t.Sprites.Tall, px, py-head, t)
			case ItemWeed:
				c.Blit(t.Sprites.Weed, px, py, t)
			case ItemDry:
				c.Blit(t.Sprites.Dry, px, py, t)
			case ItemHole:
				// A flat mark at ground level. Deliberately a different shape
				// from a dry plant, not just a different colour: in the quiet
				// theme both are the same faint grey.
				c.Rect(px+1, py+t.Sprites.Plant.H()-1, 3, 1, t.Colour(RoleSoilHole))
			case ItemPlant:
				c.Blit(t.Sprites.Plant, px, py, t)
			}
		}
	}
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	f := func(x, y uint8) uint8 { return uint8(float64(x) + (float64(y)-float64(x))*t) }
	return color.RGBA{f(a.R, b.R), f(a.G, b.G), f(a.B, b.B), 255}
}

func darken(c color.RGBA, k float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R) * k), uint8(float64(c.G) * k), uint8(float64(c.B) * k), 255}
}

// dim pulls a region towards the night sky colour, so the farm reads as dark
// without redrawing every sprite in a second palette. Unpainted pixels are left
// alone: dimming nothing would turn a transparent farm into a black rectangle.
func dim(c *Canvas, t *Theme, x, y, w, h int, k float64) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			px := c.At(x+dx, y+dy)
			if px.A == 0 {
				continue
			}
			c.Set(x+dx, y+dy, lerp(px, skyNight, k))
		}
	}
}
