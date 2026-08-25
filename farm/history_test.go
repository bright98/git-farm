package farm

import (
	"bytes"
	"image/gif"
	"testing"
	"time"

	"github.com/bright98/git-farm/internal/repo"
)

func frame(when time.Time, dirs ...*repo.Dir) repo.Frame {
	return repo.Frame{When: when, Dirs: dirs}
}

func dir(path string, files, big, weeds, dry, deleted int) *repo.Dir {
	return &repo.Dir{
		Path: path, Kind: repo.Healthy, Tests: repo.TestsFound,
		Files: files, Big: big, Weeds: weeds, Dry: dry, Deleted: deleted,
	}
}

func history() []repo.Frame {
	t0 := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	return []repo.Frame{
		frame(t0, dir("app/", 2, 0, 0, 0, 0)),
		frame(t0.AddDate(0, 1, 0), dir("app/", 8, 1, 0, 0, 0), dir("legacy/", 6, 0, 0, 0, 0)),
		frame(t0.AddDate(0, 2, 0), dir("app/", 20, 2, 3, 0, 0), dir("legacy/", 6, 0, 0, 4, 0)),
		// legacy/ is deleted: no live files, only the marks where they were.
		frame(t0.AddDate(0, 3, 0), dir("app/", 24, 2, 3, 0, 1), dir("legacy/", 0, 0, 0, 0, 6)),
	}
}

// The whole discipline of a time-lapse: fields keep their place from the first
// frame to the last, so what moves is the crop and not the ground.
func TestTheLayoutNeverMovesBetweenFrames(t *testing.T) {
	frames := history()
	s := FromFrames(frames)

	var first []Rect
	for i, f := range frames {
		s.Play(f)
		c := NewCanvas(100, 60)
		s.Draw(c, Options{Theme: &Quiet, Names: true})

		var now []Rect
		for _, field := range s.Fields {
			now = append(now, field.Bounds())
		}
		if i == 0 {
			first = now
			continue
		}
		if len(now) != len(first) {
			t.Fatalf("frame %d has %d fields, the first had %d", i, len(now), len(first))
		}
		for j := range now {
			if now[j] != first[j] {
				t.Errorf("frame %d moved field %q from %+v to %+v",
					i, s.Fields[j].Name, first[j], now[j])
			}
		}
	}
}

// A directory that grew for two months and was then deleted has no field at
// HEAD at all. Sizing the layout from HEAD would drop it, and the time-lapse
// would show a repository that had always looked roughly like it looks now.
func TestADeletedDirectoryStillGetsAField(t *testing.T) {
	s := FromFrames(history())

	found := false
	for _, f := range s.Fields {
		if f.Name == "legacy/" {
			found = true
		}
	}
	if !found {
		t.Fatal("legacy/ has no field, so its whole life is invisible")
	}

	// And by the last frame it is a field of flat marks rather than an absence.
	s.Play(history()[3])
	s.Draw(NewCanvas(100, 60), Options{Theme: &Quiet, Names: true})
	for _, f := range s.Fields {
		if f.Name != "legacy/" {
			continue
		}
		if f.Counts.Hole != 6 || f.Counts.Total() != 6 {
			t.Errorf("legacy/ ends as %+v, want six holes and nothing else", f.Counts)
		}
	}
}

// The counts have to be the frame's, not the last frame's the scene was built
// from — the failure that would make a time-lapse a still picture played over.
func TestEachFrameBringsItsOwnCrop(t *testing.T) {
	frames := history()
	s := FromFrames(frames)

	var totals []int
	for _, f := range frames {
		s.Play(f)
		s.Draw(NewCanvas(100, 60), Options{Theme: &Quiet, Names: true})
		for _, field := range s.Fields {
			if field.Name == "app/" {
				totals = append(totals, field.Counts.Total())
			}
		}
	}

	// Files is every live file, with Big and Weeds subsets of it — so a field
	// plants Files squares, plus one flat mark for each deleted file.
	want := []int{2, 8, 20, 25}
	if len(totals) != len(want) {
		t.Fatalf("app/ appeared in %d frames of %d", len(totals), len(want))
	}
	for i := range want {
		if totals[i] != want[i] {
			t.Errorf("frame %d planted %d in app/, want %d", i, totals[i], want[i])
		}
	}
}

// Several people at once, each in the field they were working in.
func TestEveryOccupiedFieldGetsAFarmer(t *testing.T) {
	frames := history()
	s := FromFrames(frames)
	s.Play(frames[2])

	c := NewCanvas(120, 80)
	s.Draw(c, Options{Theme: &Quiet, Names: true, Farmers: []string{"app/", "legacy/"}})

	occupied := s.occupied(Options{Farmers: []string{"app/", "legacy/"}})
	if len(occupied) != 2 {
		t.Fatalf("two directories were being worked in, %d fields got a farmer", len(occupied))
	}

	// A directory nobody has heard of puts nobody anywhere, rather than
	// defaulting to field zero.
	if n := len(s.occupied(Options{Farmers: []string{"nowhere/"}})); n != 0 {
		t.Errorf("an unknown directory put a farmer in %d fields", n)
	}
}

func TestGIFIsDecodableAndLoops(t *testing.T) {
	frames := history()
	s := FromFrames(frames)

	var b bytes.Buffer
	err := WriteGIF(&b, len(frames), GIFOptions{
		Theme: &Full, Cols: 80, Rows: 24, Scale: 2, Names: true,
	}, func(i int, c *Canvas) {
		s.Play(frames[i])
		s.Draw(c, Options{Theme: &Full, Names: true})
	})
	if err != nil {
		t.Fatal(err)
	}

	g, err := gif.DecodeAll(bytes.NewReader(b.Bytes()))
	if err != nil {
		t.Fatalf("the file we just wrote does not decode: %v", err)
	}
	if len(g.Image) != len(frames) {
		t.Errorf("wrote %d frames, decoded %d", len(frames), len(g.Image))
	}
	if g.LoopCount != 0 {
		t.Errorf("loop count %d, want 0 — forever", g.LoopCount)
	}
	if w, h := g.Config.Width, g.Config.Height; w != 160 || h != 96 {
		t.Errorf("decoded %dx%d, want 160x96", w, h)
	}
	// The last frame is held, so a reader sees where the history arrived
	// rather than a picture that snaps away the instant it lands.
	if last, ordinary := g.Delay[len(g.Delay)-1], g.Delay[0]; last <= ordinary {
		t.Errorf("the last frame is held %d, an ordinary one %d", last, ordinary)
	}
}

// Two runs over the same history have to produce the same bytes, for the same
// reason the SVG does: a file that changes when nothing changed gets pushed
// every night.
func TestGIFIsDeterministic(t *testing.T) {
	draw := func() []byte {
		frames := history()
		s := FromFrames(frames)
		var b bytes.Buffer
		_ = WriteGIF(&b, len(frames), GIFOptions{Theme: &Quiet, Cols: 80, Rows: 24, Scale: 2},
			func(i int, c *Canvas) {
				s.Play(frames[i])
				s.Draw(c, Options{Theme: &Quiet, Names: true})
			})
		return b.Bytes()
	}
	if a, b := draw(), draw(); !bytes.Equal(a, b) {
		t.Error("two runs over the same history produced different bytes")
	}
}
