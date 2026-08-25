// Package tui is the farm you can walk around in.
//
// The SVG is for other people: it goes in a README and answers "what shape is
// this repository" to somebody who has never seen it. This is for the person
// who works here, and it answers the next question — which file — by letting
// them point at things.
//
// It is deliberately two levels and no more. The farm, where the cursor moves
// between fields, and one field opened, where it moves between that
// directory's files. A third level would be a file browser, and there are
// better file browsers than this one.
package tui

import (
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bright98/git-farm/farm"
	"github.com/bright98/git-farm/internal/repo"
)

// A mode is which of the two levels is on screen.
type mode int

const (
	modeFarm mode = iota
	modeField
)

// Model is the whole of the state. Bubble Tea gives raw mode, key parsing, the
// alt screen and a resize message; everything below is this program's.
type Model struct {
	repo  *repo.Repo
	scene *farm.Scene

	themes  []*farm.Theme
	theme   int
	profile farm.Profile
	night   bool

	cols, rows int
	ready      bool // no size yet: the first frame waits for the window

	mode   mode
	field  int // index into scene.Fields
	file   int // index into files, when a field is open
	files  []*repo.File
	offset int // first file row on screen, for a directory taller than the window

	err  string // a refusal to draw, shown in place of the farm
	note string // an answer to a key that could not do what was asked
}

// New builds the model. The scene is laid out on the first resize, not here,
// because how many fields fit is a property of the window and there is no
// window yet.
func New(r *repo.Repo, p farm.Profile, night bool) Model {
	return Model{
		repo:    r,
		scene:   farm.FromRepo(r),
		themes:  []*farm.Theme{&farm.Quiet, &farm.Full},
		profile: p,
		night:   night,
	}
}

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.resize(msg.Width, msg.Height), nil

	case tea.KeyMsg:
		return m.key(msg)
	}
	return m, nil
}

// resize is handled from the first message rather than read once at startup,
// because a terminal that changes size mid-session is the normal case and a
// farm laid out for the old window is wrong in a way that looks like a bug.
func (m Model) resize(w, h int) Model {
	m.cols, m.rows = w, h
	m.ready = true

	if w < farm.MinCols || m.picture() < farm.MinRows {
		m.err = fmt.Sprintf("the farm needs a window of at least %d×%d; this one is %d×%d",
			farm.MinCols, farm.MinRows+chromeRows, w, h)
		return m
	}
	m.err = ""

	// Laying out here rather than in View keeps the cursor honest: the fields
	// it indexes are the ones that were just placed.
	m.scene.Draw(farm.NewCanvas(w, m.picture()*2), m.drawOptions(0))
	m.field = clamp(m.field, 0, len(m.scene.Fields)-1)
	return m
}

// chromeRows is what the farm does not get: the status line, the key hints,
// and a blank row between them and the picture.
const chromeRows = 3

func (m Model) picture() int { return m.rows - chromeRows }

func (m Model) drawOptions(cursor int) farm.Options {
	return farm.Options{
		Theme:  m.themes[m.theme],
		Night:  m.night,
		Names:  true,
		Cursor: cursor,
	}
}

func (m Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "t":
		m.theme = (m.theme + 1) % len(m.themes)
		return m, nil

	case "n":
		m.night = !m.night
		return m, nil

	case "esc", "backspace":
		if m.mode == modeField {
			m.mode, m.file, m.offset = modeFarm, 0, 0
		}
		return m, nil

	case "enter", " ":
		if m.mode == modeFarm {
			return m.open(), nil
		}
	}

	if m.mode == modeField {
		return m.fileKey(k), nil
	}
	return m.farmKey(k), nil
}

func (m Model) farmKey(k tea.KeyMsg) Model {
	dirs := map[string]farm.Direction{
		"left": farm.Left, "h": farm.Left,
		"right": farm.Right, "l": farm.Right,
		"up": farm.Up, "k": farm.Up,
		"down": farm.Down, "j": farm.Down,
	}
	if d, ok := dirs[k.String()]; ok {
		m.field, m.note = m.scene.Neighbour(m.field, d), ""
	}
	return m
}

func (m Model) fileKey(k tea.KeyMsg) Model {
	switch k.String() {
	case "down", "j":
		m.file = clamp(m.file+1, 0, len(m.files)-1)
	case "up", "k":
		m.file = clamp(m.file-1, 0, len(m.files)-1)
	case "home", "g":
		m.file = 0
	case "end", "G":
		m.file = len(m.files) - 1
	}

	// Scroll only when the cursor would otherwise leave the window, so a
	// directory that fits never scrolls at all.
	if rows := m.listRows(); rows > 0 {
		if m.file < m.offset {
			m.offset = m.file
		}
		if m.file >= m.offset+rows {
			m.offset = m.file - rows + 1
		}
	}
	return m
}

// open enters the field under the cursor, with its files worst first: the
// order the farm itself argues for, since a weed is the most useful thing the
// picture can point at.
func (m Model) open() Model {
	if m.field < 0 || m.field >= len(m.scene.Fields) {
		return m
	}
	name := m.scene.Fields[m.field].Name

	m.files = nil
	for _, f := range m.repo.Files {
		if f.Dir == name {
			m.files = append(m.files, f)
		}
	}
	if len(m.files) == 0 {
		// other/ is the one field that is not a directory: it is however many
		// were gathered together to keep the farm legible, so it has no files
		// of its own to open. Saying so beats a key that silently does nothing.
		m.note = name + " is several directories gathered together, and has no files of its own"
		return m
	}
	m.note = ""

	sort.SliceStable(m.files, func(i, j int) bool {
		a, b := m.files[i], m.files[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if a.Churn != b.Churn {
			return a.Churn > b.Churn
		}
		return a.Path < b.Path
	})

	m.mode, m.file, m.offset = modeField, 0, 0
	return m
}

// rank is the order the four marks are worth looking at in, which is the same
// order the picture decides a square by.
func rank(f *repo.File) int {
	switch {
	case f.Weed:
		return 0
	case f.Deleted:
		return 1
	case f.Dry:
		return 2
	case f.Big:
		return 3
	default:
		return 4
	}
}

func clamp(v, lo, hi int) int {
	if hi < lo {
		return lo
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func plural(n int, one string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", one)
	}
	return fmt.Sprintf("%d %ss", n, one)
}

// ago is a duration a person would say out loud, measured from the newest
// commit rather than from now — the same clock the rest of the tool uses, so
// "a year" means the same thing here as it does on the fence.
func ago(then, newest time.Time) string {
	if then.IsZero() {
		return "never, in this window"
	}
	d := newest.Sub(then)
	switch {
	case d < 36*time.Hour:
		return "today"
	case d < 14*24*time.Hour:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%d weeks ago", int(d.Hours()/24/7))
	case d < 2*365*24*time.Hour:
		return fmt.Sprintf("%d months ago", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%.0f years ago", d.Hours()/24/365)
	}
}

func trim(s string, n int) string {
	if n <= 1 || len([]rune(s)) <= n {
		return s
	}
	return string([]rune(s)[:n-1]) + "…"
}
