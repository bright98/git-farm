package farm

import (
	"math"
	"sort"

	"github.com/bright98/git-farm/internal/repo"
)

// FromRepo turns a parsed repository into a scene: which directories exist,
// how heavy each one is, and what stands in it.
//
// How many of them actually get drawn is not decided here. That depends on the
// size of the window, so Fit decides it at drawing time: a repository with 400
// directories and a window with room for eight does not draw 400 unreadable
// specks, it draws the eight biggest and gathers the rest into other/.
func FromRepo(r *repo.Repo) *Scene {
	fields := make([]Field, 0, len(r.Dirs))
	counts := countsByDir(r)

	for _, d := range r.Dirs {
		c := counts[d.Path]
		if c.Total() == 0 {
			continue
		}
		fields = append(fields, Field{
			Name:   d.Path,
			Kind:   kindOf(d.Kind),
			Fence:  fenceOf(d.Tests),
			Counts: c,
			Weight: weigh(c.Total()),
		})
	}

	// Biggest first, and a stable tie-break by name so two runs over the same
	// repository never disagree about the order.
	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Weight != fields[j].Weight {
			return fields[i].Weight > fields[j].Weight
		}
		return fields[i].Name < fields[j].Name
	})

	return &Scene{
		Fields: fields,
		all:    fields,
		Farmer: fieldNamed(fields, r.LastDir),
		Author: r.LastAuthor,
	}
}

const otherField = "other/"

// weigh turns a file count into an area, and it does it with a square root
// rather than directly.
//
// Homebrew is the argument. Library/Homebrew/ holds 2,845 of its 5,600 files
// and every other directory is under a hundred, so area in proportion to the
// count gives that one field 93% of the farm and leaves the rest as slivers
// too small to draw — a picture of one directory, from a repository that has
// forty.
//
// A field does not need room for all of its files, because it never draws all
// of them: it draws a sample that keeps the proportions. What it needs room
// for is that mix, and that grows far more slowly than the count does. The
// square root keeps the order — a bigger directory is always a bigger field —
// and gives the smaller ones enough space to say anything at all.
func weigh(files int) float64 {
	if files <= 0 {
		return 0
	}
	return math.Sqrt(float64(files))
}

// countsByDir sorts each directory's files into what they will be drawn as.
//
// The order is the one documented on Counts: a file that is several things at
// once is drawn as the most alarming of them.
func countsByDir(r *repo.Repo) map[string]Counts {
	out := map[string]Counts{}
	for _, f := range r.Files {
		c := out[f.Dir]
		switch {
		case f.Deleted:
			c.Hole++
		case f.Weed:
			c.Weed++
		case f.Dry:
			c.Dry++
		case f.Big:
			c.Tall++
		default:
			c.Plain++
		}
		out[f.Dir] = c
	}
	return out
}

// gather keeps the biggest fields and folds the rest into one.
//
// The alternative — drawing every directory — produces a mosaic in which no
// single field is big enough to show a plant, which loses the only thing the
// picture is for.
func gather(fields []Field, keep int) []Field {
	if len(fields) <= keep {
		return fields
	}

	rest := fields[keep-1:]
	other := Field{
		Name: otherField,
		Kind: Dead,
		// A bag of directories is not a claim about tests. Anything else here
		// would be a fence drawn over a dozen different answers.
		Fence: FenceUnknown,
	}
	for _, f := range rest {
		other.Counts.Plain += f.Counts.Plain
		other.Counts.Tall += f.Counts.Tall
		other.Counts.Weed += f.Counts.Weed
		other.Counts.Dry += f.Counts.Dry
		other.Counts.Hole += f.Counts.Hole
		if f.Kind != Dead {
			other.Kind = Healthy
		}
	}
	other.Weight = weigh(other.Counts.Total())

	return append(fields[:keep-1:keep-1], other)
}

func kindOf(k repo.Kind) Kind {
	switch k {
	case repo.Hotspot:
		return Hotspot
	case repo.Untested:
		return Untested
	case repo.Dead:
		return Dead
	default:
		return Healthy
	}
}

func fenceOf(t repo.TestState) Fence {
	switch t {
	case repo.TestsFound:
		return FenceSolid
	case repo.TestsMissing:
		return FenceBroken
	default:
		return FenceUnknown
	}
}
