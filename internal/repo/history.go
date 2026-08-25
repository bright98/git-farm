package repo

import (
	"errors"
	"sort"
	"time"

	"github.com/bright98/git-farm/internal/gitlog"
)

// A Frame is the repository as it stood at one moment, carrying only what the
// picture needs: the fields, and who was working in them.
//
// It deliberately does not carry files. A time-lapse of a repository with five
// thousand files over two hundred frames would hold a million file records to
// draw a few hundred plants, and every one of the plants comes from a tally
// the directory already has.
type Frame struct {
	When    time.Time
	Commits int // commits in the history up to and including this frame
	Active  int // commits inside this frame's own window

	Dirs    []*Dir
	Farmers []Farmer

	// Late is set when a commit in this window was made in the small hours by
	// its author's own clock. It is what lights the lantern.
	Late bool
}

// A Farmer is somebody who committed inside a frame's window, and the field
// they were last working in.
type Farmer struct {
	Name string
	Dir  string
}

// Cadence is how much history one frame covers. It is chosen from the shape of
// the repository rather than asked for, because the right answer is different
// for a month-old project and a fifteen-year-old one, and nobody wants to tune
// it by hand.
type Cadence string

const (
	PerCommit Cadence = "commit"
	PerDay    Cadence = "day"
	PerWeek   Cadence = "week"
	PerMonth  Cadence = "month"
)

// A Timeline is the whole history, cut into frames.
type Timeline struct {
	Cadence Cadence
	Frames  []Frame
}

// MaxFarmers is the cap the plan puts on how many people are drawn at once.
// Beyond it the farm is a crowd and the fields are behind them.
const MaxFarmers = 8

// History replays the log into frames.
//
// Two passes over the log, not one. The first reads nothing but timestamps —
// no numstat, so it is cheap — and that is what makes the cadence a decision
// taken before the replay rather than during it. Deciding as it went would mean
// discovering at commit forty thousand that every frame so far was the wrong
// size.
func History(dir string, cfg Config, opts Options, maxFrames int) (*Timeline, error) {
	cfg = cfg.normalise()
	if maxFrames < 2 {
		maxFrames = 2
	}

	info, err := gitlog.Inspect(dir)
	if err != nil {
		return nil, err
	}
	logOpts := gitlog.Options{MaxCommits: opts.MaxCommits}
	if opts.Since > 0 {
		logOpts.Since = info.When.Add(-opts.Since.Duration()).Format(time.RFC3339)
	}

	times, err := gitlog.Times(info.Root, logOpts)
	if err != nil {
		return nil, err
	}
	if len(times) < 2 {
		return nil, ErrTooShort
	}

	cadence, bounds := cut(times, maxFrames)
	head, err := gitlog.HeadFiles(info.Root)
	if err != nil {
		return nil, err
	}

	// The thresholds come from the whole history, once, and every frame is
	// measured against them. Re-ranking each frame against itself would make a
	// file big in one frame and ordinary in the next without anything having
	// happened to it, and the farm would flicker with news that is not news.
	full, err := Build(dir, cfg, opts)
	if err != nil {
		return nil, err
	}
	bigAt, weedAt := thresholds(full, cfg)

	t := &Timeline{Cadence: cadence}
	acc := newAccumulator()
	next := 0

	err = gitlog.Walk(info.Root, logOpts, func(c gitlog.Commit) error {
		if err := acc.add(c); err != nil {
			return err
		}
		for next < len(bounds) && !c.When.Before(bounds[next]) {
			t.Frames = append(t.Frames, acc.frame(info.Root, c.When, head, cfg, bigAt, weedAt))
			next++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// The last frame is HEAD however the boundaries fell, so a time-lapse always
	// ends on the farm the still picture would have drawn.
	if len(t.Frames) == 0 || t.Frames[len(t.Frames)-1].Commits < acc.commits {
		t.Frames = append(t.Frames, acc.frame(info.Root, acc.last, head, cfg, bigAt, weedAt))
	}

	t.markFarmers(acc, cfg)
	return t, nil
}

// cut chooses the cadence and returns the moment each frame ends.
//
// The plan's rule first — one commit each for a small repository, one active
// day for a normal one, one week for a very old one — and then a cap, because
// the rule alone gives a fifteen-year-old repository nine hundred frames and a
// GIF nobody can put in a README. When the rule overflows, whole buckets are
// merged rather than dropped: a frame covering four days is still true, where a
// frame that skipped three of them is not.
func cut(times []time.Time, maxFrames int) (Cadence, []time.Time) {
	span := times[len(times)-1].Sub(times[0])

	cadence := PerDay
	switch {
	case len(times) < 500:
		cadence = PerCommit
	case span > 10*365*24*time.Hour:
		cadence = PerWeek
	}

	bounds := bucket(times, cadence)
	for len(bounds) > maxFrames {
		switch cadence {
		case PerCommit:
			cadence = PerDay
		case PerDay:
			cadence = PerWeek
		case PerWeek:
			cadence = PerMonth
		default:
			// Already as coarse as the calendar goes. Merge whole buckets,
			// evenly, keeping the last: the end of the history is the farm
			// everybody recognises.
			step := (len(bounds) + maxFrames - 1) / maxFrames
			var kept []time.Time
			for i := len(bounds) - 1; i >= 0; i -= step {
				kept = append(kept, bounds[i])
			}
			sort.Slice(kept, func(i, j int) bool { return kept[i].Before(kept[j]) })
			return cadence, kept
		}
		bounds = bucket(times, cadence)
	}
	return cadence, bounds
}

// bucket is the end of each frame's window, oldest first.
func bucket(times []time.Time, c Cadence) []time.Time {
	if c == PerCommit {
		out := make([]time.Time, len(times))
		copy(out, times)
		return out
	}

	var out []time.Time
	var last string
	for _, t := range times {
		key := ""
		switch c {
		case PerDay:
			key = t.Format("2006-01-02")
		case PerWeek:
			y, w := t.ISOWeek()
			key = time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006") + "-" + itoa(w)
		default:
			key = t.Format("2006-01")
		}
		if key != last {
			out = append(out, t)
			last = key
		} else {
			out[len(out)-1] = t // the window ends at its newest commit
		}
	}
	return out
}

func itoa(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// thresholds is what "big" and "churned" mean in this repository, taken once
// from the whole of it.
func thresholds(r *Repo, cfg Config) (bigAt, weedAt int) {
	var lines, churn []int
	for _, f := range r.Files {
		if f.Deleted {
			continue
		}
		lines = append(lines, f.Lines)
		churn = append(churn, f.Churn)
	}
	return topThreshold(lines, cfg.BigFiles), topThreshold(churn, cfg.Churn)
}

// frame is the state of the accumulator, rolled up into fields.
//
// Line counts here are what the history added and took away again, never what
// is on disk: a frame is a moment that is not checked out, and counting the
// lines of a past commit would mean checking out every one of them. It only
// decides plant height, which the README already calls the weakest of the four
// marks.
func (a *accumulator) frame(root string, when time.Time, head map[string]bool, cfg Config, bigAt, weedAt int) Frame {
	f := Frame{When: when, Commits: a.commits}
	quiet := when.Add(-cfg.Quiet.Duration())

	byDir := map[string]*Dir{}
	churn := map[string][]int{}
	paths := map[string][]string{}

	for p, s := range a.stats {
		if s.first.After(when) {
			continue // not written yet
		}
		// A path missing from HEAD was deleted at some point, and its last
		// touch in the log is the commit that deleted it. Before that it was a
		// file like any other; after it, a flat mark.
		gone := !head[p] && !s.last.After(when)

		dir := fieldOf(p, cfg.Depth)
		d := byDir[dir]
		if d == nil {
			d = &Dir{Path: dir}
			byDir[dir] = d
		}

		if gone {
			d.Deleted++
			continue
		}
		paths[dir] = append(paths[dir], p)

		lines := max(0, s.added-s.deleted)
		c := s.commits * len(s.authors)

		d.Files++
		d.Lines += lines
		churn[dir] = append(churn[dir], c)
		if s.last.After(d.Last) {
			d.Last, d.Author = s.last, s.lastAuthor
		}

		switch {
		case weedAt > 0 && c >= weedAt:
			d.Weeds++
		case s.last.Before(quiet):
			d.Dry++
		case bigAt > 0 && lines >= bigAt:
			d.Big++
		}
	}

	hot := topThreshold(medians(churn), cfg.DirChurn)
	for dir, d := range byDir {
		d.Churn = median(churn[dir])
		d.Tests = testState(root, paths[dir], cfg)
		d.Kind = frameKind(d, hot, cfg)
		f.Dirs = append(f.Dirs, d)
	}
	sort.Slice(f.Dirs, func(i, j int) bool { return f.Dirs[i].Path < f.Dirs[j].Path })
	return f
}

func medians(churn map[string][]int) []float64 {
	out := make([]float64, 0, len(churn))
	for _, v := range churn {
		out = append(out, median(v))
	}
	return out
}

// frameKind is the ladder rollUp climbs, against a frame's own clock rather
// than against HEAD's. The two have to agree, or the last frame of a
// time-lapse would disagree with the still picture of the same commit.
func frameKind(d *Dir, hotAt float64, cfg Config) Kind {
	minFiles := cfg.HotspotFiles
	if minFiles < 1 {
		minFiles = 1
	}
	switch {
	case d.Files == 0 || d.Dry == d.Files:
		return Dead
	case hotAt > 0 && d.Files >= minFiles && d.Churn >= hotAt:
		return Hotspot
	case d.Tests == TestsMissing:
		return Untested
	default:
		return Healthy
	}
}

// markFarmers fills in who was working in each frame's window.
//
// Only the people who committed inside the window, capped, and each standing
// in the field they last touched. A farmer per author over the whole history
// would put a crowd on a farm that one person has worked alone for a year.
func (t *Timeline) markFarmers(acc *accumulator, cfg Config) {
	for i := range t.Frames {
		from := time.Time{}
		if i > 0 {
			from = t.Frames[i-1].When
		}
		f := &t.Frames[i]

		// One farmer per person, standing where they last worked in this
		// window — not one per commit, or a busy week is the same person
		// drawn forty times.
		where := map[string]string{}
		var order []string
		for _, w := range acc.work {
			if w.When.After(f.When) || !w.When.After(from) {
				continue
			}
			f.Active++
			if w.Late {
				f.Late = true
			}
			if _, seen := where[w.Author]; !seen {
				order = append(order, w.Author)
			}
			where[w.Author] = fieldOf(w.Path, cfg.Depth)
		}

		for _, who := range order {
			if len(f.Farmers) >= MaxFarmers {
				break
			}
			f.Farmers = append(f.Farmers, Farmer{Name: who, Dir: where[who]})
		}
	}
}

// ErrTooShort is a history with nothing to play: one commit is a still
// picture, and a time-lapse of it would be the same frame twice.
var ErrTooShort = errors.New("too little history to play: a time-lapse needs at least two commits")
