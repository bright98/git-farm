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
			dirs:   []string{d.Path},
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
		other.dirs = append(other.dirs, f.dirs...)
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

// Play pours one moment of the history into a scene whose layout is already
// decided.
//
// It writes into the scene's own list of directories rather than into the
// fields the window was given, because those are rebuilt from it on every
// draw: how many fields fit is a property of the window, and gathering the
// leftovers into other/ has to happen after the counts are in, not before.
//
// The weights are not touched. That is the whole discipline of a time-lapse
// here: fields keep their place from the first frame to the last, so what moves
// is the crop and not the ground. Re-weighing per frame would make every new
// directory re-flow the farm, and the eye would follow the rearranging rather
// than the growing.
func (s *Scene) Play(f repo.Frame) {
	byDir := make(map[string]*repo.Dir, len(f.Dirs))
	for _, d := range f.Dirs {
		byDir[d.Path] = d
	}

	for i := range s.all {
		field := &s.all[i]
		d := byDir[field.Name]
		if d == nil {
			// A directory nobody has written yet, or one whose every file has
			// gone. Empty, but still a field: its place is kept.
			field.Counts = Counts{}
			field.Kind, field.Fence = Dead, FenceUnknown
			continue
		}

		plain := d.Files - d.Big - d.Weeds - d.Dry
		if plain < 0 {
			plain = 0 // a file can be big and churned at once
		}
		field.Counts = Counts{
			Plain: plain, Tall: d.Big, Weed: d.Weeds, Dry: d.Dry, Hole: d.Deleted,
		}
		field.Kind, field.Fence = kindOf(d.Kind), fenceOf(d.Tests)
	}
	s.Fields = nil // rebuilt from s.all by the next Fit
}

// FromFrames builds the scene a time-lapse plays into.
//
// A field for every directory the history ever had, sized by the biggest it
// ever was rather than by what is left of it. Sizing from HEAD would be the
// obvious thing and it would hide the most interesting half of a history: a
// directory that grew for two years and was deleted last month has no field at
// HEAD at all, so it would never appear, grow, or die — the picture would show
// a repository that had always looked roughly like it looks now.
func FromFrames(frames []repo.Frame) *Scene {
	peak := map[string]int{}
	kind := map[string]repo.Kind{}
	tests := map[string]repo.TestState{}

	for _, f := range frames {
		for _, d := range f.Dirs {
			if n := d.Files + d.Deleted; n > peak[d.Path] {
				peak[d.Path] = n
			}
			kind[d.Path], tests[d.Path] = d.Kind, d.Tests
		}
	}

	fields := make([]Field, 0, len(peak))
	for path, n := range peak {
		if n == 0 {
			continue
		}
		fields = append(fields, Field{
			Name:   path,
			Kind:   kindOf(kind[path]),
			Fence:  fenceOf(tests[path]),
			Weight: weigh(n),
			dirs:   []string{path},
		})
	}

	sort.SliceStable(fields, func(i, j int) bool {
		if fields[i].Weight != fields[j].Weight {
			return fields[i].Weight > fields[j].Weight
		}
		return fields[i].Name < fields[j].Name
	})

	return &Scene{Fields: fields, all: fields, Farmer: -1}
}
