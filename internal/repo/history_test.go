package repo

import (
	"testing"
	"time"
)

func times(n int, step time.Duration) []time.Time {
	base := time.Date(2020, 1, 1, 12, 0, 0, 0, time.UTC)
	out := make([]time.Time, n)
	for i := range out {
		out[i] = base.Add(time.Duration(i) * step)
	}
	return out
}

// The cadence comes from the shape of the repository, and then the cap comes
// from the shape of a GIF: the plan's rule alone gives an old repository nine
// hundred frames.
func TestCadenceFollowsTheRepositoryThenTheCap(t *testing.T) {
	cases := []struct {
		name    string
		times   []time.Time
		max     int
		cadence Cadence
	}{
		{"a young repository gets a frame per commit", times(40, time.Hour), 120, PerCommit},
		{"a busy one falls back to days", times(900, 3*time.Hour), 120, PerDay},
		{"a long one to weeks or wider", times(900, 30*24*time.Hour), 120, PerMonth},
	}

	for _, c := range cases {
		cadence, bounds := cut(c.times, c.max)
		if cadence != c.cadence {
			t.Errorf("%s: chose %s", c.name, cadence)
		}
		if len(bounds) > c.max {
			t.Errorf("%s: %d frames, cap was %d", c.name, len(bounds), c.max)
		}
		if len(bounds) == 0 {
			t.Errorf("%s: no frames at all", c.name)
		}
		for i := 1; i < len(bounds); i++ {
			if !bounds[i].After(bounds[i-1]) {
				t.Errorf("%s: frame %d does not come after %d", c.name, i, i-1)
			}
		}
	}
}

// Even at the coarsest the calendar goes, a very long history can overflow. It
// is then whole buckets that are merged, never dropped — and the last one is
// kept, because the end of the history is the farm everybody recognises.
func TestTheCapIsNeverExceededAndTheEndIsKept(t *testing.T) {
	ts := times(4000, 24*time.Hour) // eleven years, every day
	_, bounds := cut(ts, 20)

	if len(bounds) > 20 {
		t.Fatalf("%d frames from a cap of 20", len(bounds))
	}
	if last, want := bounds[len(bounds)-1], ts[len(ts)-1]; !last.Equal(want) {
		t.Errorf("the last frame is %v, want the newest commit %v", last, want)
	}
}

func TestHistoryPlaysARealRepository(t *testing.T) {
	r := newTestRepo(t)

	r.write("internal/ui/view.go", "package ui\n")
	r.write("internal/ui/view_test.go", "package ui\n")
	r.add(".")
	r.commit("Ada", "ada@example.com", 3*year, "add the ui")

	r.write("cmd/tool/main.go", "package main\n")
	r.add(".")
	r.commit("Bo", "bo@example.com", 2*year, "add the command")

	for i := 0; i < 4; i++ {
		r.write("internal/store/db.go", "package store\n// "+time.Now().String()+"\n")
		r.add(".")
		r.commit("Cy", "cy@example.com", time.Duration(300-i*30)*24*time.Hour, "work on the store")
	}

	tl, err := History(r.dir, Defaults(), Options{}, 120)
	if err != nil {
		t.Fatal(err)
	}
	if len(tl.Frames) < 2 {
		t.Fatalf("played %d frames", len(tl.Frames))
	}

	// The history only ever grows, so the commit count is never allowed to go
	// backwards — the surest sign a replay has lost its place.
	for i := 1; i < len(tl.Frames); i++ {
		if tl.Frames[i].Commits < tl.Frames[i-1].Commits {
			t.Errorf("frame %d has %d commits, after %d",
				i, tl.Frames[i].Commits, tl.Frames[i-1].Commits)
		}
	}

	// The first frame is early history: the store does not exist yet.
	for _, d := range tl.Frames[0].Dirs {
		if d.Path == "internal/store/" && d.Files > 0 {
			t.Error("the store has files in the first frame, before it was written")
		}
	}

	// And the last frame is HEAD, which has to agree with the still picture of
	// the same commit — a time-lapse that ends somewhere else is a time-lapse
	// of a different repository.
	last := tl.Frames[len(tl.Frames)-1]
	still, err := Build(r.dir, Defaults(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if last.Commits != still.Commits {
		t.Errorf("the last frame has %d commits, the still picture %d", last.Commits, still.Commits)
	}

	live := map[string]int{}
	for _, d := range last.Dirs {
		live[d.Path] = d.Files
	}
	for _, d := range still.Dirs {
		if live[d.Path] != d.Files {
			t.Errorf("%s: %d files in the last frame, %d in the still picture",
				d.Path, live[d.Path], d.Files)
		}
	}
}

// Only the people who were working in this window, and never more than the cap.
func TestFarmersAreTheOnesWorkingNow(t *testing.T) {
	r := newTestRepo(t)

	// Ada alone, long ago.
	r.write("a/one.go", "package a\n")
	r.add(".")
	r.commit("Ada", "ada@example.com", 3*year, "start")

	// Then a crowd, all at once, in different directories.
	for i, who := range []string{"Bo", "Cy", "Di", "Ed", "Fi", "Gus", "Hal", "Ivy", "Jo", "Kit"} {
		r.write("d"+string(rune('0'+i))+"/f.go", "package d\n")
		r.add(".")
		r.commit(who, who+"@example.com", 24*time.Hour, "everyone at once")
	}

	tl, err := History(r.dir, Defaults(), Options{}, 120)
	if err != nil {
		t.Fatal(err)
	}

	for i, f := range tl.Frames {
		if len(f.Farmers) > MaxFarmers {
			t.Errorf("frame %d draws %d farmers, cap is %d", i, len(f.Farmers), MaxFarmers)
		}
		seen := map[string]bool{}
		for _, w := range f.Farmers {
			if seen[w.Name] {
				t.Errorf("frame %d has %s in it twice", i, w.Name)
			}
			seen[w.Name] = true
		}
	}

	// Ada worked once, three years ago. She cannot be standing in the last
	// frame: the farmers are who is working now, not who ever worked here.
	last := tl.Frames[len(tl.Frames)-1]
	for _, w := range last.Farmers {
		if w.Name == "Ada" {
			t.Error("Ada is still standing in the field three years after her only commit")
		}
	}
}

func TestHistoryRefusesASingleCommit(t *testing.T) {
	r := newTestRepo(t)
	r.write("a.go", "package a\n")
	r.add(".")
	r.commit("Ada", "ada@example.com", 0, "the only commit")

	if _, err := History(r.dir, Defaults(), Options{}, 120); err != ErrTooShort {
		t.Errorf("a one-commit repository gave %v", err)
	}
}
