package tui

import (
	"fmt"
	"strings"

	"github.com/bright98/git-farm/farm"
)

// View is one frame. Bubble Tea diffs it against the last one line by line,
// which is close to worst case for pixel art — when the cursor moves, both the
// field it left and the field it arrived at change. It is fine for a farm that
// only redraws on a keystroke, and the plan puts a per-cell diff in phase 6,
// where a time-lapse would actually need it.
func (m Model) View() string {
	if !m.ready {
		return "" // no window size yet; drawing now would guess at one
	}
	if m.err != "" {
		return "\n  " + m.err + "\n\n  q to quit\n"
	}

	body := m.farmView()
	if m.mode == modeField {
		body = m.fieldView()
	}

	// One place decides the height. Both views used to pad themselves and one
	// of them got it wrong, which moved the status line up the screen whenever
	// a short directory was opened.
	var b strings.Builder
	b.WriteString(fit(body, m.picture()))
	b.WriteString("\n\n")
	b.WriteString(m.status())
	b.WriteString("\n")
	b.WriteString(m.hints())
	return b.String()
}

// fit makes a block exactly n lines: padded with blanks, or cut. Cutting is
// the safety net rather than the plan — a view that overflows has already got
// its own arithmetic wrong — but a frame taller than the window scrolls the
// farm out of the top, which is worse than losing its last row.
func fit(s string, n int) string {
	lines := strings.Split(s, "\n")
	for len(lines) < n {
		lines = append(lines, "")
	}
	return strings.Join(lines[:n], "\n")
}

func (m Model) farmView() string {
	return m.scene.Render(m.cols, m.picture(), m.drawOptions(m.field+1), m.profile)
}

// listRows is how many file rows the window has room for: the picture's rows,
// less the two the header and its rule take.
func (m Model) listRows() int { return m.picture() - 2 }

func (m Model) fieldView() string {
	t := m.themes[m.theme]
	paint := func(s string, r farm.Role) string { return m.profile.Paint(s, t.Colour(r)) }

	var b strings.Builder
	name := m.scene.Fields[m.field].Name

	// The columns are sized from the window, not from the longest path: a
	// narrow window should lose the end of a name rather than the numbers,
	// which are what the row is for.
	const numbers = 34
	pathW := clamp(m.cols-numbers-4, 12, 80)

	b.WriteString("  " + paint(name, farm.RoleHat))
	b.WriteString(paint(fmt.Sprintf("  %s, worst first", plural(len(m.files), "file")), farm.RoleLabel))
	b.WriteString("\n  " + paint(strings.Repeat("─", clamp(m.cols-4, 8, pathW+numbers)), farm.RoleFence) + "\n")

	rows := m.listRows()
	for i := m.offset; i < len(m.files) && i < m.offset+rows; i++ {
		f := m.files[i]

		mark, role := " ", farm.RoleLeaf
		switch {
		case f.Weed:
			mark, role = "✽", farm.RoleWeed
		case f.Deleted:
			mark, role = "▁", farm.RoleSoilHole
		case f.Dry:
			mark, role = "⌄", farm.RoleDry
		case f.Big:
			mark, role = "♣", farm.RoleHat
		default:
			mark, role = "✦", farm.RoleLeaf
		}

		cursor := "  "
		if i == m.file {
			cursor = m.profile.Paint("▸ ", t.Colour(farm.RoleHat))
		}

		path := strings.TrimPrefix(f.Path, name)
		line := fmt.Sprintf("%-*s %5d lines  %3d ✎  %2d ✍  %s",
			pathW, trim(path, pathW), f.Lines, f.Commits, f.Authors,
			ago(f.Last, m.repo.Last))

		b.WriteString(cursor + m.profile.Paint(mark, t.Colour(role)) + " ")
		if i == m.file {
			b.WriteString(m.profile.Paint(line, t.Colour(farm.RoleLabel)))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}

	return strings.TrimRight(b.String(), "\n")
}

// status is the one line that says what is under the cursor. On the farm it
// describes a directory; inside one it describes a file, which is the whole
// reason this exists rather than a picture.
func (m Model) status() string {
	t := m.themes[m.theme]
	label := func(s string) string { return m.profile.Paint(s, t.Colour(farm.RoleLabel)) }

	if m.mode == modeField {
		if len(m.files) == 0 {
			return label("  nothing in this field")
		}
		f := m.files[m.file]
		what := []string{}
		if f.Weed {
			what = append(what, "churned")
		}
		if f.Big {
			what = append(what, "big")
		}
		if f.Dry {
			what = append(what, "quiet for a year")
		}
		if f.Deleted {
			what = append(what, "deleted")
		}
		say := ""
		if len(what) > 0 {
			say = " — " + strings.Join(what, ", ")
		}
		return label(fmt.Sprintf("  %s%s · %s by %s · churn %d",
			trim(f.Path, m.cols/2), say,
			plural(f.Commits, "commit"), plural(f.Authors, "author"), f.Churn))
	}

	if m.note != "" {
		return label("  " + m.note)
	}
	if m.field < 0 || m.field >= len(m.scene.Fields) {
		return label("  no fields")
	}
	f := m.scene.Fields[m.field]
	c := f.Counts
	parts := []string{plural(c.Total(), "file")}
	for _, p := range []struct {
		n int
		s string
	}{{c.Weed, "churned"}, {c.Tall, "big"}, {c.Dry, "quiet"}, {c.Hole, "deleted"}} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.s))
		}
	}
	return label(fmt.Sprintf("  %s — %s, %s · %s",
		f.Name, kindWord(f.Kind), fenceWord(f.Fence), strings.Join(parts, ", ")))
}

func (m Model) hints() string {
	t := m.themes[m.theme]
	keys := "  ←↑↓→ move · enter open · t theme · n night · q quit"
	if m.mode == modeField {
		keys = "  ↑↓ move · esc back · t theme · n night · q quit"
	}
	return m.profile.Paint(keys, t.Colour(farm.RoleFence))
}

func kindWord(k farm.Kind) string {
	switch k {
	case farm.Hotspot:
		return "a hotspot"
	case farm.Untested:
		return "untested"
	case farm.Dead:
		return "dead"
	default:
		return "healthy"
	}
}

func fenceWord(f farm.Fence) string {
	switch f {
	case farm.FenceBroken:
		return "no test files found"
	case farm.FenceUnknown:
		return "no rule here, so no claim"
	default:
		return "tests found"
	}
}
