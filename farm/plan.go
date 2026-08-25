package farm

import (
	"hash/fnv"
	"math/rand"
	"sort"
)

// An Item is what stands in one square of a field.
type Item byte

const (
	ItemBare Item = iota
	ItemPlant
	ItemTall
	ItemWeed
	ItemDry
	ItemHole
)

// Counts is a field's files, sorted into what they will be drawn as.
//
// A file can be several things at once — big and churned and old — and a square
// holds one plant, so the order below decides. Churn wins over everything
// except deletion: a file five people have been fighting over is the most
// useful thing the picture can point at, and it stays the most useful thing
// even if they have since stopped.
//
//	deleted -> a flat mark
//	churn   -> a weed
//	quiet   -> a dry plant
//	big     -> a tall plant
//	the rest-> a plant
type Counts struct {
	Plain, Tall, Weed, Dry, Hole int
}

func (c Counts) Total() int { return c.Plain + c.Tall + c.Weed + c.Dry + c.Hole }

// plant decides what stands in each square of a field.
//
// When a field has more files than squares — and a directory of 2,000 files
// gets maybe twenty squares — the squares are shared out in proportion, with
// one rule on top: a kind that exists at all gets at least one square. The
// picture will over-state two weeds among three hundred files, and that is the
// intended direction to be wrong in. A farm that hides the weeds is no use.
func plant(seed string, c Counts, squares int) []Item {
	items := make([]Item, 0, squares)
	if squares <= 0 {
		return items
	}
	// An empty field still returns a square for every square, all of them bare.
	// Returning nothing instead used to be the same thing in practice, because
	// a directory with no files never became a field — until a time-lapse,
	// where a field exists before anybody has written a file in it.

	kinds := []struct {
		item Item
		n    int
	}{
		{ItemWeed, c.Weed},
		{ItemHole, c.Hole},
		{ItemDry, c.Dry},
		{ItemTall, c.Tall},
		{ItemPlant, c.Plain},
	}

	total := c.Total()
	if total <= squares {
		// Everything fits: one square per file, and the rest stays bare soil.
		for _, k := range kinds {
			for i := 0; i < k.n; i++ {
				items = append(items, k.item)
			}
		}
	} else {
		items = apportion(kinds, squares)
	}

	// The empty squares go in before the shuffle, not after it. Shuffling first
	// and padding afterwards packs every plant into the top of the field and
	// leaves the bottom bare, which reads as a half-drawn picture; scattering
	// the gaps instead makes a field with few files look like what it is, a
	// field with few files in it.
	for len(items) < squares {
		items = append(items, ItemBare)
	}

	// A deterministic scatter. Without it the kinds come out in blocks — every
	// weed in one corner — which reads as a layout rather than as a texture.
	// With a fixed seed the same repository always draws the same farm, which
	// is what keeps the Action from pushing a new file on every run.
	rng := rand.New(rand.NewSource(seedOf(seed)))
	rng.Shuffle(len(items), func(i, j int) { items[i], items[j] = items[j], items[i] })

	return items[:squares]
}

// apportion shares squares out by largest remainder, after reserving one square
// for every kind that exists.
func apportion(kinds []struct {
	item Item
	n    int
}, squares int) []Item {
	total := 0
	present := 0
	for _, k := range kinds {
		total += k.n
		if k.n > 0 {
			present++
		}
	}

	got := make([]int, len(kinds))
	left := squares

	// The reservation. If there are more kinds present than squares, the rarest
	// kinds are the ones to keep: they are listed first for that reason.
	for i, k := range kinds {
		if k.n > 0 && left > 0 {
			got[i], left = 1, left-1
		}
	}

	// The proportional share of what is left, by largest remainder so the parts
	// always add up to exactly the number of squares.
	type share struct {
		idx  int
		frac float64
	}
	var fracs []share
	assigned := 0
	for i, k := range kinds {
		exact := float64(k.n) / float64(total) * float64(left)
		whole := int(exact)
		got[i] += whole
		assigned += whole
		fracs = append(fracs, share{i, exact - float64(whole)})
	}

	sort.SliceStable(fracs, func(a, b int) bool { return fracs[a].frac > fracs[b].frac })
	for i := 0; assigned < left; i++ {
		got[fracs[i%len(fracs)].idx]++
		assigned++
	}

	out := make([]Item, 0, squares)
	for i, k := range kinds {
		for j := 0; j < got[i] && len(out) < squares; j++ {
			out = append(out, k.item)
		}
	}
	return out
}

// seedOf turns a field's name into a stable seed. Never the global rand: two
// runs would disagree and the Action would push a new picture every time.
func seedOf(s string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return int64(h.Sum64() &^ (1 << 63))
}
