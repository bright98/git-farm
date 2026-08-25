package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bright98/git-farm/farm"
	"github.com/bright98/git-farm/internal/repo"
)

var now = time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)

// sample is a repository with one of everything: a tested package, a hotspot
// with a churned file in it, a package with no tests, and a directory nothing
// can be claimed about.
func sample() *repo.Repo {
	file := func(dir, name string, lines, commits, authors int, set func(*repo.File)) *repo.File {
		f := &repo.File{
			Path: dir + name, Dir: dir,
			Lines: lines, Commits: commits, Authors: authors,
			Churn: commits * authors,
			Last:  now.Add(-24 * time.Hour),
		}
		if set != nil {
			set(f)
		}
		return f
	}

	r := &repo.Repo{
		Root: "/tmp/sample", Head: "abc", Commits: 40, Authors: 3, Last: now,
		LastAuthor: "Ada", LastDir: "internal/store/",
		Dirs: []*repo.Dir{
			{Path: "internal/ui/", Kind: repo.Healthy, Tests: repo.TestsFound, Files: 6},
			{Path: "internal/store/", Kind: repo.Hotspot, Tests: repo.TestsFound, Files: 4, Weeds: 1},
			{Path: "cmd/tool/", Kind: repo.Untested, Tests: repo.TestsMissing, Files: 3},
			{Path: "docs/", Kind: repo.Healthy, Tests: repo.TestsUnknown, Files: 3},
		},
	}

	for i, n := range []string{"view.go", "render.go", "input.go", "theme.go", "keys.go", "help.go"} {
		r.Files = append(r.Files, file("internal/ui/", n, 40+i*10, 2, 1, nil))
	}
	r.Files = append(r.Files,
		file("internal/store/", "db.go", 900, 20, 3, func(f *repo.File) { f.Weed, f.Big = true, true }),
		file("internal/store/", "db_test.go", 120, 4, 2, nil),
		file("internal/store/", "cache.go", 80, 2, 1, nil),
		file("internal/store/", "gone.go", 0, 3, 1, func(f *repo.File) { f.Deleted = true }),
	)
	for i, n := range []string{"main.go", "flags.go", "run.go"} {
		r.Files = append(r.Files, file("cmd/tool/", n, 30+i*5, 1, 1, nil))
	}
	for i, n := range []string{"guide.md", "rules.md", "old.md"} {
		r.Files = append(r.Files, file("docs/", n, 100+i, 1, 1, func(f *repo.File) {
			if n == "old.md" {
				f.Dry = true
				f.Last = now.Add(-3 * 365 * 24 * time.Hour)
			}
		}))
	}
	return r
}

// start is a model that has already been told how big the window is, which is
// the only state in which it will draw anything.
func start(t *testing.T, w, h int) Model {
	t.Helper()
	m, _ := New(sample(), farm.Mono, false).Update(tea.WindowSizeMsg{Width: w, Height: h})
	return m.(Model)
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "right":
			msg = tea.KeyMsg{Type: tea.KeyRight}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

// Nothing is drawn before the window has said how big it is. Guessing a size
// and redrawing is how a farm laid out for eighty columns ends up wrapped in a
// hundred and twenty.
func TestNothingIsDrawnBeforeTheFirstResize(t *testing.T) {
	m := New(sample(), farm.Mono, false)
	if got := m.View(); got != "" {
		t.Errorf("drew %q before knowing the window size", got)
	}
}

func TestRefusesAWindowTooSmallToRead(t *testing.T) {
	m := start(t, 40, 12)
	if !strings.Contains(m.View(), "needs a window of at least") {
		t.Errorf("a 40x12 window drew a farm instead of saying it cannot:\n%s", m.View())
	}

	// And recovers when the window grows, rather than staying refused.
	m = start(t, 120, 40)
	if strings.Contains(m.View(), "needs a window") {
		t.Error("still refusing after the window grew")
	}
}

func TestTheCursorMovesBetweenFields(t *testing.T) {
	m := start(t, 120, 40)
	first := m.field

	moved := press(t, m, "right")
	if moved.field == first {
		// Not every farm has a field to the right of the first one, so try the
		// other three before calling it stuck.
		for _, k := range []string{"down", "left", "up"} {
			if press(t, m, k).field != first {
				return
			}
		}
		t.Error("the cursor cannot reach any other field")
	}
}

// Whichever way the cursor goes, it has to land on a field that is drawn.
// Moving onto one the window had no room for is a cursor that vanishes.
func TestTheCursorOnlyLandsOnDrawnFields(t *testing.T) {
	m := start(t, 120, 40)
	for _, k := range []string{"right", "down", "left", "up", "down", "right", "up", "left"} {
		m = press(t, m, k)
		if m.field < 0 || m.field >= len(m.scene.Fields) {
			t.Fatalf("after %q the cursor is on field %d of %d", k, m.field, len(m.scene.Fields))
		}
		if m.scene.Fields[m.field].Bounds().W == 0 {
			t.Fatalf("after %q the cursor is on %q, which is not drawn",
				k, m.scene.Fields[m.field].Name)
		}
	}
}

func TestEnterOpensTheFieldAndEscapeLeaves(t *testing.T) {
	m := start(t, 120, 40)
	name := m.scene.Fields[m.field].Name

	m = press(t, m, "enter")
	if m.mode != modeField {
		t.Fatal("enter did not open the field")
	}
	if len(m.files) == 0 {
		t.Fatalf("%s opened with no files", name)
	}
	for _, f := range m.files {
		if f.Dir != name {
			t.Errorf("%s is in the list for %s", f.Path, name)
		}
	}

	if back := press(t, m, "esc"); back.mode != modeFarm {
		t.Error("escape did not go back to the farm")
	}
}

// Worst first, because the point of the tool is that the bad corner finds you.
func TestFilesAreSortedWorstFirst(t *testing.T) {
	m := start(t, 120, 40)
	for i := range m.scene.Fields {
		m.field = i
		if m.scene.Fields[i].Name != "internal/store/" {
			continue
		}
		m = press(t, m, "enter")

		if got := m.files[0].Path; got != "internal/store/db.go" {
			t.Errorf("the churned file is not first, %q is", got)
		}
		last := -1
		for _, f := range m.files {
			if r := rank(f); r < last {
				t.Errorf("%s (rank %d) comes after rank %d", f.Path, r, last)
			} else {
				last = r
			}
		}
		return
	}
	t.Skip("the store field was not drawn at this size")
}

func TestTheCursorStaysInsideTheFileList(t *testing.T) {
	m := press(t, start(t, 120, 40), "enter")
	n := len(m.files)

	up := press(t, m, "up", "up", "up")
	if up.file != 0 {
		t.Errorf("moving up from the first file landed on %d", up.file)
	}

	down := m
	for i := 0; i < n+5; i++ {
		down = press(t, down, "down")
	}
	if down.file != n-1 {
		t.Errorf("moving past the last of %d files landed on %d", n, down.file)
	}
}

func TestThemeAndNightToggle(t *testing.T) {
	m := start(t, 120, 40)
	if next := press(t, m, "t"); next.theme == m.theme {
		t.Error("t did not change the theme")
	}
	if next := press(t, m, "t", "t"); next.theme != m.theme {
		t.Error("t does not come back round to where it started")
	}
	if next := press(t, m, "n"); next.night == m.night {
		t.Error("n did not toggle night")
	}
}

func TestQuitAsks(t *testing.T) {
	m := start(t, 120, 40)
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil {
		t.Error("q returned no command, so the program never quits")
	}
}

// The status line is the whole reason this is not a picture: whatever the
// cursor is on, the bottom of the screen says what it is.
func TestTheStatusLineNamesWhatIsUnderTheCursor(t *testing.T) {
	m := start(t, 120, 40)

	if name := m.scene.Fields[m.field].Name; !strings.Contains(m.View(), name) {
		t.Errorf("the farm does not name the field under the cursor (%s)", name)
	}

	m = press(t, m, "enter")
	if path := m.files[m.file].Path; !strings.Contains(m.View(), path) {
		t.Errorf("the open field does not name the file under the cursor (%s)", path)
	}
}

// Every frame has to be the height of the window, or the status line walks up
// and down the screen as directories of different sizes are opened.
func TestEveryFrameIsTheWindowHeight(t *testing.T) {
	const h = 40
	m := start(t, 120, h)

	for _, name := range []string{"the farm", "an open field", "a smaller field"} {
		lines := strings.Count(m.View(), "\n") + 1
		if lines != h {
			t.Errorf("%s is %d lines in a %d-line window", name, lines, h)
		}
		m = press(t, m, "enter")
	}
}

func TestDumpFrames(t *testing.T) {
	m := start(t, 100, 32)
	t.Log("\n--- the farm ---\n" + m.View())
	t.Log("\n--- a field opened ---\n" + press(t, m, "enter").View())
}

// The cursor cannot be colour alone. --no-color, a dumb TERM and a pipe all
// paint nothing, and a farm you cannot find your cursor in is not navigable.
func TestTheCursorIsVisibleWithoutColour(t *testing.T) {
	m := start(t, 120, 40)
	if !strings.Contains(m.View(), "▸") {
		t.Error("no cursor mark in a farm drawn with no colour at all")
	}

	// And it is on the field the status line is talking about.
	name := m.scene.Fields[m.field].Name
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.Contains(line, "▸") && strings.Contains(line, strings.TrimSuffix(name, "/")) {
			return
		}
	}
	t.Errorf("the cursor mark is not on %s, which the status line names", name)
}

// other/ is the one field that is not a directory, so there is nothing to
// open. A key that silently does nothing reads as a broken key.
func TestOpeningTheGatheredFieldSaysWhyItCannot(t *testing.T) {
	// More directories than a farm draws, so some are gathered into other/.
	r := sample()
	for i := 0; i < 20; i++ {
		dir := fmt.Sprintf("pkg%02d/", i)
		r.Dirs = append(r.Dirs, &repo.Dir{Path: dir, Kind: repo.Healthy, Tests: repo.TestsFound, Files: 1})
		r.Files = append(r.Files, &repo.File{Path: dir + "a.go", Dir: dir, Lines: 10, Commits: 1, Authors: 1, Last: now})
	}
	mm, _ := New(r, farm.Mono, false).Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m := mm.(Model)

	found := false
	for i, f := range m.scene.Fields {
		if f.Name != "other/" {
			continue
		}
		m.field = i
		m = press(t, m, "enter")

		if m.mode == modeField {
			t.Fatal("other/ opened as if it were a directory")
		}
		if !strings.Contains(m.View(), "gathered together") {
			t.Errorf("nothing said why other/ did not open:\n%s", m.status())
		}
		found = true
		break
	}
	if !found {
		t.Fatal("twenty extra directories produced no other/ field to test")
	}
}
