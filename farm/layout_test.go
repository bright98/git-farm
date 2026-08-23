package farm

import (
	"strings"
	"testing"

	"github.com/haleh/git-farm/internal/repo"
)

// Every tile has to stay inside the area and out of its neighbours. An overlap
// is two fields drawing over each other's fences.
func TestSquarifyTilesDoNotOverlap(t *testing.T) {
	area := Rect{2, 10, 100, 50}
	weights := []float64{40, 22, 18, 17, 9, 5, 3, 2}

	tiles := Squarify(area, weights)

	for i, r := range tiles {
		if r.X < area.X || r.Y < area.Y || r.X+r.W > area.X+area.W || r.Y+r.H > area.Y+area.H {
			t.Errorf("tile %d escaped the area: %+v not inside %+v", i, r, area)
		}
	}

	for i := 0; i < len(tiles); i++ {
		for j := i + 1; j < len(tiles); j++ {
			if overlap(tiles[i], tiles[j]) {
				t.Errorf("tiles %d %+v and %d %+v overlap", i, tiles[i], j, tiles[j])
			}
		}
	}
}

func overlap(a, b Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

// Bigger weight, bigger field. The exact areas are the treemap's business, but
// the order has to survive or the picture is lying about which directory is
// which.
func TestSquarifyKeepsTheOrderOfSizes(t *testing.T) {
	area := Rect{0, 0, 120, 60}
	weights := []float64{50, 25, 12, 6}

	tiles := Squarify(area, weights)
	for i := 1; i < len(tiles); i++ {
		if tiles[i-1].Area() < tiles[i].Area() {
			t.Errorf("weight %v got %d pixels but weight %v got %d",
				weights[i-1], tiles[i-1].Area(), weights[i], tiles[i].Area())
		}
	}

	// And the whole area is used, give or take the rounding.
	total := 0
	for _, r := range tiles {
		total += r.Area()
	}
	if got := float64(total) / float64(area.Area()); got < 0.9 {
		t.Errorf("the tiles cover only %.0f%% of the area", got*100)
	}
}

// A field's border is drawn one rune per cell, and a cell is two pixels tall.
// Two fields whose borders land in the same cell row would overwrite each
// other, so the gutter has to be at least a whole cell.
func TestInsetLeavesAWholeCellBetweenFields(t *testing.T) {
	for y := 10; y < 20; y++ {
		r := Inset(Rect{X: 4, Y: y, W: 30, H: 20})
		if r.Y%2 != 0 {
			t.Errorf("Inset(y=%d) starts at an odd pixel %d: its border would share a cell row", y, r.Y)
		}
		if r.H%2 != 0 {
			t.Errorf("Inset(y=%d) has an odd height %d", y, r.H)
		}

		below := Inset(Rect{X: 4, Y: y + 20, W: 30, H: 20})
		lastCell := (r.Y + r.H - 1) / 2
		firstCell := below.Y / 2
		if firstCell <= lastCell {
			t.Errorf("fields touch: one ends in cell row %d and the next starts in %d", lastCell, firstCell)
		}
	}
}

// A directory that will not fit is gathered into other/, never quietly
// dropped. Losing the dead corner of a repository is losing the point.
func TestFitGathersRatherThanDropping(t *testing.T) {
	var fields []Field
	for _, n := range []int{400, 90, 80, 30, 20, 12, 9, 7, 5, 4, 3, 2} {
		fields = append(fields, Field{
			Name:   strings.Repeat("d", n%5+1) + "/",
			Counts: Counts{Plain: n},
			Weight: weigh(n),
		})
	}

	before := 0
	for _, f := range fields {
		before += f.Counts.Total()
	}

	s := &Scene{Fields: fields, all: fields, Farmer: -1}
	s.Fit(Rect{2, 14, 90, 40}, &Quiet)

	if s.Drawn() == 0 {
		t.Fatal("nothing was placed at all")
	}
	if s.Drawn() != len(s.Fields) {
		t.Errorf("%d fields were kept but only %d are big enough to draw",
			len(s.Fields), s.Drawn())
	}

	after := 0
	for _, f := range s.Fields {
		after += f.Counts.Total()
	}
	if after != before {
		t.Errorf("%d files went into the layout and %d came out", before, after)
	}
}

// Laying the same scene out twice must give the same farm. Otherwise the
// Action pushes a new picture every run, for a repository that has not changed.
func TestFitIsRepeatable(t *testing.T) {
	build := func() *Scene {
		var fields []Field
		for i, n := range []int{300, 80, 40, 22, 11, 6, 3} {
			fields = append(fields, Field{
				Name:   string(rune('a'+i)) + "/",
				Counts: Counts{Plain: n},
				Weight: weigh(n),
			})
		}
		return &Scene{Fields: fields, all: fields, Farmer: -1}
	}

	area := Rect{2, 14, 90, 44}

	once := build()
	once.Fit(area, &Quiet)

	twice := build()
	twice.Fit(area, &Quiet)
	twice.Fit(area, &Quiet) // and again, on a scene that has already been fitted

	if len(once.Fields) != len(twice.Fields) {
		t.Fatalf("laying out twice gave %d fields, once gave %d", len(twice.Fields), len(once.Fields))
	}
	for i := range once.Fields {
		if once.Fields[i] != twice.Fields[i] {
			t.Errorf("field %d differs: %+v then %+v", i, once.Fields[i], twice.Fields[i])
		}
	}
}

// The squares are shared out in proportion, with one rule on top: a kind that
// exists at all keeps at least one square. Two weeds among three hundred files
// is exactly the case the picture must not hide.
func TestPlantKeepsProportionsAndNeverHidesAKind(t *testing.T) {
	c := Counts{Plain: 300, Weed: 2, Dry: 40, Tall: 8, Hole: 1}
	items := plant("internal/store/", c, 40)

	if len(items) != 40 {
		t.Fatalf("got %d squares, want 40", len(items))
	}

	got := map[Item]int{}
	for _, it := range items {
		got[it]++
	}

	for _, kind := range []Item{ItemWeed, ItemHole, ItemDry, ItemTall, ItemPlant} {
		if got[kind] == 0 {
			t.Errorf("a kind that exists in the directory got no square at all: %v", kind)
		}
	}
	if got[ItemBare] != 0 {
		t.Errorf("a full field left %d squares bare", got[ItemBare])
	}

	// The big proportions still have to be roughly right.
	if share := float64(got[ItemPlant]) / 40; share < 0.5 || share > 0.9 {
		t.Errorf("plain files are %.0f%% of the directory but %.0f%% of the field",
			300.0/351*100, share*100)
	}
	if got[ItemDry] < 2 {
		t.Errorf("40 quiet files out of 351 got only %d squares", got[ItemDry])
	}
}

// A field with room to spare shows its files spread out, not packed into the
// top with a bare half underneath.
func TestPlantScattersTheGaps(t *testing.T) {
	items := plant("pkg/", Counts{Plain: 6}, 20)

	last := -1
	for i, it := range items {
		if it != ItemBare {
			last = i
		}
	}
	if last < 10 {
		t.Errorf("every plant landed in the first half of the field (last at %d of %d)", last, len(items))
	}
}

func TestPlantIsDeterministic(t *testing.T) {
	a := plant("internal/store/", Counts{Plain: 12, Weed: 4, Dry: 2}, 24)
	b := plant("internal/store/", Counts{Plain: 12, Weed: 4, Dry: 2}, 24)

	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("two runs planted the same field differently at square %d", i)
		}
	}

	// A different field is a different arrangement, or every field on the farm
	// looks like a copy of the one beside it.
	if other := plant("migrations/", Counts{Plain: 12, Weed: 4, Dry: 2}, 24); equalItems(a, other) {
		t.Error("two different directories were planted identically")
	}
}

func equalItems(a, b []Item) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A file can be several things at once, and a square holds one plant.
func TestCountsFollowThePriority(t *testing.T) {
	r := &repo.Repo{
		Dirs: []*repo.Dir{{Path: "pkg/", Kind: repo.Healthy, Tests: repo.TestsFound}},
		Files: []*repo.File{
			{Path: "pkg/gone.go", Dir: "pkg/", Deleted: true, Weed: true, Big: true},
			{Path: "pkg/hot.go", Dir: "pkg/", Weed: true, Dry: true, Big: true},
			{Path: "pkg/old.go", Dir: "pkg/", Dry: true, Big: true},
			{Path: "pkg/big.go", Dir: "pkg/", Big: true},
			{Path: "pkg/ok.go", Dir: "pkg/"},
		},
	}

	got := FromRepo(r).Fields[0].Counts
	want := Counts{Hole: 1, Weed: 1, Dry: 1, Tall: 1, Plain: 1}
	if got != want {
		t.Errorf("counts %+v, want %+v", got, want)
	}
}

// The farmer stands where the newest commit landed — and when that directory is
// too small for a field of its own, in the field that swallowed it.
func TestFarmerFollowsTheNewestCommit(t *testing.T) {
	r := &repo.Repo{
		LastAuthor: "Ada",
		LastDir:    "tools/",
		Dirs: []*repo.Dir{
			{Path: "internal/", Tests: repo.TestsFound},
			{Path: "tools/", Tests: repo.TestsMissing},
		},
		Files: []*repo.File{
			{Path: "internal/a.go", Dir: "internal/"},
			{Path: "tools/b.go", Dir: "tools/"},
		},
	}

	s := FromRepo(r)
	if s.Farmer < 0 || s.Fields[s.Farmer].Name != "tools/" {
		t.Fatalf("the farmer is in %d, want the tools/ field", s.Farmer)
	}

	// Now squeeze the farm until tools/ is gathered away.
	s.Fields = gather(s.all, 1)
	s.Farmer = fieldNamed(s.Fields, "tools/")
	if s.Farmer < 0 || s.Fields[s.Farmer].Name != otherField {
		t.Errorf("the farmer vanished with their directory instead of moving to %s", otherField)
	}
}

// A field's name is worth more as its last segment than as its first: nobody
// recognises "internal…", everybody recognises "store/".
func TestShorten(t *testing.T) {
	cases := []struct {
		name string
		room int
		want string
	}{
		{"internal/store/", 20, "internal/store/"},
		{"internal/store/", 10, "store/"},
		{"git-farm-demo/farm/", 8, "farm/"},
		{"averyveryverylongname/", 8, "averyve…"},
		{"./", 4, "./"},
	}
	for _, c := range cases {
		if got := shorten(c.name, c.room); got != c.want {
			t.Errorf("shorten(%q, %d) = %q, want %q", c.name, c.room, got, c.want)
		}
		if got := shorten(c.name, c.room); len([]rune(got)) > c.room {
			t.Errorf("shorten(%q, %d) = %q, which is too long", c.name, c.room, got)
		}
	}
}

// A bigger directory is always a bigger field, however the weights are
// compressed.
func TestWeighIsMonotonic(t *testing.T) {
	prev := 0.0
	for _, n := range []int{1, 2, 5, 20, 100, 2845} {
		w := weigh(n)
		if w <= prev {
			t.Errorf("weigh(%d) = %v, which is not more than the previous %v", n, w, prev)
		}
		prev = w
	}
	if weigh(0) != 0 {
		t.Error("an empty directory should weigh nothing")
	}

	// And the compression has to actually compress, or Homebrew's one huge
	// directory takes the whole farm again.
	if ratio := weigh(2845) / weigh(87); ratio > 8 {
		t.Errorf("the biggest directory outweighs a middling one by %.0f×", ratio)
	}
}

func TestMaxFieldsGrowsWithTheWindow(t *testing.T) {
	small := MaxFields(Rect{0, 0, 40, 20}, &Quiet)
	big := MaxFields(Rect{0, 0, 200, 100}, &Quiet)

	if small < 1 {
		t.Error("even the smallest window must allow one field")
	}
	if big <= small {
		t.Errorf("a bigger window allowed %d fields, a smaller one %d", big, small)
	}
	if big > 10 {
		t.Errorf("%d fields is a mosaic, not a farm", big)
	}
}

// A field that is drawn at all has to be plantable by the theme drawing it.
// The failure this guards is silent: the layout is calibrated for the quiet
// terminal's four-pixel plants, and the themes that write a file grow plants
// half again as tall — so a directory with files in it came out of the SVG as
// a fenced rectangle with a name on it and nothing inside, while the terminal
// drew the same directory with a row of plants in it.
func TestEveryDrawnFieldCanBePlanted(t *testing.T) {
	for _, theme := range []*Theme{&Quiet, &QuietLight, &Full, forFile(&Quiet)} {
		_, minH := theme.MinField()
		head := theme.Sprites.Tall.H() - theme.Sprites.Plant.H()

		for _, size := range [][2]int{{MinCols, MinRows}, {80, 24}, {120, 36}, {200, 50}} {
			_, area := Layout(size[0], size[1]*2)
			s := sample()
			s.Fit(area, theme)

			for i, f := range s.Fields {
				r := f.Bounds()
				if r.W == 0 {
					continue // not drawn at all, which is the honest answer
				}

				reserve := 0
				if i == s.Farmer {
					_, _, reserve = farmerFor(theme, r)
				}
				// The sum plantField does: the soil left over, less the room a
				// tall plant needs above the row, over the row spacing.
				if rows := (r.H - 4 - reserve - head) / theme.StepY; rows < 1 {
					t.Errorf("%s %dx%d: %s is %dx%d and holds no plants (minimum height %d, farmer takes %d)",
						theme.Name, size[0], size[1], f.Name, r.W, r.H, minH, reserve)
				}
			}
		}
	}
}
