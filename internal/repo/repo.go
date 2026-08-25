// Package repo turns a stream of commits into the four kinds of field the farm
// draws: healthy, hotspot, untested, dead.
//
// This is the part that has to be right. The drawing is decoration on top of
// three claims that carry real weight — weeds are churn, fences are tests, dry
// plants are code nobody has touched in years — and each of those is decided
// here.
package repo

import (
	"bytes"
	"math"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bright98/git-farm/internal/gitlog"
)

// Kind is what a directory looks like from above. A directory can be several of
// these at once; the picture has room for one, so they are checked in order:
// dead, then hotspot, then untested.
type Kind string

const (
	Healthy  Kind = "healthy"
	Hotspot  Kind = "hotspot"  // constant churn: weeds
	Untested Kind = "untested" // no test files found: a broken fence
	Dead     Kind = "dead"     // untouched for a year or more: dry plants
)

// File is one path and what its history says about it.
type File struct {
	Path string `json:"path"`
	Dir  string `json:"dir"` // the field it stands in, after the depth limit

	Lines   int `json:"lines"`   // counted at HEAD; 0 for binary and deleted files
	Commits int `json:"commits"` // within the window --since kept
	Authors int `json:"authors"` // distinct email addresses
	Churn   int `json:"churn"`   // commits × authors: the hotspot measure

	First time.Time `json:"first_seen,omitzero"`
	Last  time.Time `json:"last_seen,omitzero"`

	// A file that exists at HEAD but has no commit inside the window is
	// recorded with Commits: 0 and no timestamps. It is not new — it is old
	// enough that --since cut its whole history off, so it counts as quiet.
	Deleted bool `json:"deleted,omitempty"`
	Big     bool `json:"big,omitempty"`  // in the top slice by line count
	Weed    bool `json:"weed,omitempty"` // in the top slice by churn
	Dry     bool `json:"dry,omitempty"`  // untouched for longer than Config.Quiet
}

// Dir is a field on the farm: a directory, cut off at the configured depth,
// with everything underneath rolled up into it.
type Dir struct {
	Path  string    `json:"path"`
	Kind  Kind      `json:"kind"`
	Tests TestState `json:"tests"`

	Files   int `json:"files"` // live files, the ones that get a plant
	Deleted int `json:"deleted"`
	Big     int `json:"big"`
	Weeds   int `json:"weeds"`
	Dry     int `json:"dry"`

	Lines  int       `json:"lines"`
	Churn  float64   `json:"churn"` // the median file churn under here
	Last   time.Time `json:"last_seen,omitzero"`
	Author string    `json:"last_author,omitempty"`
}

// Repo is everything the renderers need, and the whole of what --json prints.
type Repo struct {
	Root string `json:"root"`
	Head string `json:"head"`

	Commits int       `json:"commits"`
	Authors int       `json:"authors"`
	First   time.Time `json:"first_commit,omitzero"`
	Last    time.Time `json:"last_commit,omitzero"`

	// LastAuthor is where the farmer stands: the author of the newest commit
	// that changed a file, in the field that commit touched. A repo whose head
	// is an empty release commit still has a farmer, one commit further back.
	LastAuthor string `json:"last_author,omitempty"`
	LastDir    string `json:"last_dir,omitempty"`

	// LastChange is when that commit was made, in its author's own timezone.
	// It is the farmer's clock: the one commit the picture is already about,
	// so if the farm is drawn at night it is that person's night.
	LastChange time.Time `json:"last_change,omitzero"`

	Config Config  `json:"config"`
	Dirs   []*Dir  `json:"dirs"`
	Files  []*File `json:"files"`
}

// Options are the two ways to read less history than there is.
type Options struct {
	Since      Span // measured back from the newest commit, not from today
	MaxCommits int
}

// Build reads the repository at dir and classifies it.
func Build(dir string, cfg Config, opts Options) (*Repo, error) {
	cfg = cfg.normalise()

	info, err := gitlog.Inspect(dir)
	if err != nil {
		return nil, err
	}

	// --since counts back from the newest commit rather than from now. Two runs
	// a week apart over the same HEAD then produce the same farm, which is what
	// makes the Action's output byte-identical and its push a no-op.
	logOpts := gitlog.Options{MaxCommits: opts.MaxCommits}
	if opts.Since > 0 {
		logOpts.Since = info.When.Add(-opts.Since.Duration()).Format(time.RFC3339)
	}

	acc := newAccumulator()
	if err := gitlog.Walk(info.Root, logOpts, acc.add); err != nil {
		return nil, err
	}

	head, err := gitlog.HeadFiles(info.Root)
	if err != nil {
		return nil, err
	}

	r := &Repo{
		Root:       info.Root,
		Head:       info.Head,
		Commits:    acc.commits,
		Authors:    len(acc.authors),
		First:      acc.first,
		Last:       acc.last,
		LastAuthor: acc.lastAuthor,
		Config:     cfg,
	}
	r.Files = acc.files(head, info.Root, cfg)
	r.classify(cfg)
	r.rollUp(info.Root, head, cfg, acc)
	return r, nil
}

// accumulator folds the commit stream into one record per path as it goes.
// Streaming matters: a repo with 50,000 commits is a million change records,
// and only the totals are ever needed.
type accumulator struct {
	stats   map[string]*stat
	authors map[string]bool
	commits int

	first, last time.Time

	// The farmer stands where the newest commit that changed a file landed,
	// which is not always the newest commit. A release commit or a merge that
	// touches nothing names a person but no field, and a farmer with nowhere
	// to stand is drawn nowhere at all — so the picture would quietly lose the
	// only thing in it that is about a person, and the SVG its only moving
	// part. The newest commit's own time still sets the clock; only the person
	// and the place come from the newest commit that did something.
	touched      time.Time
	touchedLocal time.Time
	lastAuthor   string
	lastPaths    []string

	// work is one entry per commit that changed something: who, when, and
	// where. The still picture needs only the newest of these; a time-lapse
	// needs all of them, because every frame has its own people in it.
	work []Work
}

// Work is one commit that changed a file: who made it, when, and the field it
// landed in.
type Work struct {
	Author string
	When   time.Time
	Path   string // a file the commit touched; the field is decided later, from the depth
	Late   bool   // made in the small hours, by the author's own clock
}

type stat struct {
	commits        int
	authors        map[string]bool
	added, deleted int
	first, last    time.Time
	lastAuthor     string
}

func newAccumulator() *accumulator {
	return &accumulator{stats: map[string]*stat{}, authors: map[string]bool{}}
}

func (a *accumulator) add(c gitlog.Commit) error {
	a.commits++
	a.authors[strings.ToLower(c.Email)] = true

	if a.first.IsZero() || c.When.Before(a.first) {
		a.first = c.When
	}
	if c.When.After(a.last) || a.last.IsZero() {
		a.last = c.When
	}
	if len(c.Changes) > 0 {
		h := c.Local.Hour()
		a.work = append(a.work, Work{
			Author: c.Author,
			When:   c.When,
			Path:   c.Changes[0].Path,
			Late:   h >= 0 && h < 6,
		})
	}
	if len(c.Changes) > 0 && (c.When.After(a.touched) || a.touched.IsZero()) {
		a.touched, a.touchedLocal = c.When, c.Local
		a.lastAuthor, a.lastPaths = c.Author, nil
		for _, ch := range c.Changes {
			a.lastPaths = append(a.lastPaths, ch.Path)
		}
	}

	for _, ch := range c.Changes {
		// A rename carries the file's history with it. Walking oldest-first is
		// what makes this a single move: by the time the new name appears, all
		// of the old name's history is already gathered under it.
		if ch.OldPath != "" {
			if old, ok := a.stats[ch.OldPath]; ok {
				delete(a.stats, ch.OldPath)
				a.merge(ch.Path, old)
			}
		}

		s := a.stats[ch.Path]
		if s == nil {
			s = &stat{authors: map[string]bool{}, first: c.When}
			a.stats[ch.Path] = s
		}
		s.commits++
		s.authors[strings.ToLower(c.Email)] = true
		s.added += ch.Added
		s.deleted += ch.Deleted
		if s.first.IsZero() || c.When.Before(s.first) {
			s.first = c.When
		}
		if c.When.After(s.last) {
			s.last, s.lastAuthor = c.When, c.Author
		}
	}
	return nil
}

func (a *accumulator) merge(into string, old *stat) {
	s := a.stats[into]
	if s == nil {
		a.stats[into] = old
		return
	}
	s.commits += old.commits
	for e := range old.authors {
		s.authors[e] = true
	}
	s.added += old.added
	s.deleted += old.deleted
	if s.first.IsZero() || (!old.first.IsZero() && old.first.Before(s.first)) {
		s.first = old.first
	}
	if old.last.After(s.last) {
		s.last, s.lastAuthor = old.last, old.lastAuthor
	}
}

// files turns the accumulated stats into File records, and adds the files that
// exist at HEAD but never appeared in the window --since kept.
func (a *accumulator) files(head map[string]bool, root string, cfg Config) []*File {
	out := make([]*File, 0, len(a.stats))

	for p, s := range a.stats {
		f := &File{
			Path:    p,
			Dir:     fieldOf(p, cfg.Depth),
			Commits: s.commits,
			Authors: len(s.authors),
			Churn:   s.commits * len(s.authors),
			First:   s.first,
			Last:    s.last,
			Deleted: !head[p],
		}
		if f.Deleted {
			// Nothing to count on disk. What the history added and took away
			// again is the best guess left, and it only decides plant height.
			f.Lines = max(0, s.added-s.deleted)
		} else {
			f.Lines = countLines(filepath.Join(root, p))
		}
		out = append(out, f)
	}

	// A file older than the window still stands in the field. Leaving it out
	// would empty a long-lived repo the moment somebody passed --since 1y.
	for p := range head {
		if _, seen := a.stats[p]; seen {
			continue
		}
		out = append(out, &File{
			Path:  p,
			Dir:   fieldOf(p, cfg.Depth),
			Lines: countLines(filepath.Join(root, p)),
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// classify marks the big files, the weeds and the dry ones.
//
// Everything here is a rank inside this repository, never a fixed number. Forty
// commits is nothing in one repo and a crisis in another, and a tool that uses
// absolutes tells small projects they are perfect and large ones that they are
// doomed.
func (r *Repo) classify(cfg Config) {
	var lines, churn []int
	for _, f := range r.Files {
		if f.Deleted {
			continue // a deleted file is a flat mark; it is neither big nor weedy
		}
		lines = append(lines, f.Lines)
		churn = append(churn, f.Churn)
	}

	bigAt := topThreshold(lines, cfg.BigFiles)
	weedAt := topThreshold(churn, cfg.Churn)
	quiet := r.Last.Add(-cfg.Quiet.Duration())

	for _, f := range r.Files {
		if f.Deleted {
			continue
		}
		f.Big = bigAt > 0 && f.Lines >= bigAt
		f.Weed = weedAt > 0 && f.Churn >= weedAt
		// No commit in the window means the file is older than the window, so
		// it has certainly been quiet for longer than the window.
		f.Dry = f.Last.IsZero() || f.Last.Before(quiet)
	}
}

// rollUp gathers files into fields and decides what each field is.
func (r *Repo) rollUp(root string, head map[string]bool, cfg Config, acc *accumulator) {
	type bucket struct {
		dir   *Dir
		churn []int
		paths []string // every live path under here, for the test rules
	}

	buckets := map[string]*bucket{}
	for _, f := range r.Files {
		b := buckets[f.Dir]
		if b == nil {
			b = &bucket{dir: &Dir{Path: f.Dir}}
			buckets[f.Dir] = b
		}

		if f.Deleted {
			b.dir.Deleted++
		} else {
			b.dir.Files++
			b.dir.Lines += f.Lines
			b.churn = append(b.churn, f.Churn)
			b.paths = append(b.paths, f.Path)
		}
		if f.Big {
			b.dir.Big++
		}
		if f.Weed {
			b.dir.Weeds++
		}
		if f.Dry && !f.Deleted {
			b.dir.Dry++
		}
		if f.Last.After(b.dir.Last) {
			b.dir.Last = f.Last
			if s := acc.stats[f.Path]; s != nil {
				b.dir.Author = s.lastAuthor
			}
		}
	}

	dirs := make([]*Dir, 0, len(buckets))
	for _, b := range buckets {
		b.dir.Churn = median(b.churn)
		b.dir.Tests = testState(root, b.paths, cfg)
		dirs = append(dirs, b.dir)
	}

	// Only directories with enough files are ranked against each other. A
	// median taken over one file is that file, so without this every
	// single-file directory in a large repo outranks the code — bin/ and
	// completions/fish/ come out as the hotspots and the busy package does not.
	// If nothing clears the bar, the repo is small enough that one file is all
	// the evidence there is, so rank everything.
	medians := make([]float64, 0, len(dirs))
	for _, d := range dirs {
		if d.Files >= cfg.HotspotFiles {
			medians = append(medians, d.Churn)
		}
	}
	minFiles := cfg.HotspotFiles
	if len(medians) == 0 {
		minFiles = 1
		for _, d := range dirs {
			medians = append(medians, d.Churn)
		}
	}

	hotAt := topThreshold(medians, cfg.DirChurn)

	for _, d := range dirs {
		switch {
		// Nothing alive here has been touched in a year. A directory with only
		// deleted files counts too: it is the emptiest kind of dead.
		case d.Files == 0 || d.Dry == d.Files:
			d.Kind = Dead
		case hotAt > 0 && d.Files >= minFiles && d.Churn >= hotAt:
			d.Kind = Hotspot
		case d.Tests == TestsMissing:
			d.Kind = Untested
		default:
			d.Kind = Healthy
		}
	}

	sort.Slice(dirs, func(i, j int) bool { return dirs[i].Path < dirs[j].Path })
	r.Dirs = dirs

	// The farmer stands where the newest commit landed.
	if len(acc.lastPaths) > 0 {
		r.LastDir = fieldOf(acc.lastPaths[0], cfg.Depth)
	}
	r.LastChange = acc.touchedLocal
}

// fieldOf cuts a path down to the field it belongs to. Files at the top level
// share one field named "./", because a farm with a one-file field per README
// is not a picture of anything.
func fieldOf(p string, depth int) string {
	dir := path.Dir(p)
	if dir == "." || dir == "/" {
		return "./"
	}
	parts := strings.Split(dir, "/")
	if len(parts) > depth {
		parts = parts[:depth]
	}
	return strings.Join(parts, "/") + "/"
}

// topThreshold returns the value at the start of the top share of vals: with
// share 0.10, the smallest value in the top 10%.
//
// The guard matters more than the percentile. When every file has the same
// churn — a young repo where one person made every commit once — the top 10%
// is still 10% of the files, and the farm sprouts weeds that mean nothing. So a
// value must also beat the middle of the distribution to count.
//
// A zero return means "no threshold": nothing here stands out.
func topThreshold[T int | float64](vals []T, share float64) T {
	if len(vals) == 0 {
		return 0
	}

	sorted := append([]T(nil), vals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	i := int(math.Ceil(float64(len(sorted)) * (1 - share)))
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	if i < 0 {
		i = 0
	}

	at := sorted[i]
	if mid := sorted[len(sorted)/2]; at <= mid {
		return 0 // too flat a distribution to call anything a peak
	}
	return at
}

func median(vals []int) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := append([]int(nil), vals...)
	sort.Ints(sorted)

	n := len(sorted)
	if n%2 == 1 {
		return float64(sorted[n/2])
	}
	return float64(sorted[n/2-1]+sorted[n/2]) / 2
}

// countLines counts the lines of a file as it stands at HEAD.
//
// Line count is a weak measure of importance — a 40-line file can matter more
// than a 4,000-line one — and it decides nothing except how tall a plant is.
// The claims that carry weight are churn, silence and tests.
func countLines(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	// A binary file has no lines worth drawing, and its bytes would make it the
	// tallest plant in the repo.
	probe := data
	if len(probe) > 8000 {
		probe = probe[:8000]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		return 0
	}

	n := bytes.Count(data, []byte{'\n'})
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++
	}
	return n
}
