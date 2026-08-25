package gitlog

import (
	"strings"
	"testing"
	"time"
)

// The log stream, written the way git writes it: a header marked with \x01, a
// blank line, then one numstat line per file.
func log(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

func TestParseCommits(t *testing.T) {
	in := log(
		"\x01abc123|Ada|ada@example.com|1700000000|2023-11-15T01:43:20+03:30|add the store",
		"",
		"12\t3\tinternal/store/matches.go",
		"5\t0\tinternal/store/matches_test.go",
		"",
		"\x01def456|Bo|bo@example.com|1700086400|2023-11-16T01:43:20+03:30|tidy up",
		"",
		"1\t1\tREADME.md",
	)

	var got []Commit
	if err := parse(strings.NewReader(in), func(c Commit) error {
		got = append(got, c)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d commits, want 2", len(got))
	}

	first := got[0]
	if first.Hash != "abc123" || first.Author != "Ada" || first.Email != "ada@example.com" {
		t.Errorf("header parsed as %+v", first)
	}
	if first.Subject != "add the store" {
		t.Errorf("subject %q", first.Subject)
	}
	if want := time.Unix(1700000000, 0).UTC(); !first.When.Equal(want) {
		t.Errorf("time %v, want %v", first.When, want)
	}
	if len(first.Changes) != 2 {
		t.Fatalf("got %d changes, want 2", len(first.Changes))
	}
	if c := first.Changes[0]; c.Path != "internal/store/matches.go" || c.Added != 12 || c.Deleted != 3 {
		t.Errorf("change parsed as %+v", c)
	}

	// UTC everywhere, or two runs in different timezones disagree about which
	// day a commit happened on and the Action pushes a new file every time.
	if name, _ := got[1].When.Zone(); name != "UTC" {
		t.Errorf("commit time is in %s, want UTC", name)
	}
}

// A subject can contain anything, including the separator and something that
// looks exactly like a numstat line. The \x01 marker is what keeps them apart.
func TestParseSubjectWithSeparators(t *testing.T) {
	in := log(
		"\x01abc|Ada|ada@example.com|1700000000|2023-11-15T01:43:20+03:30|fix a|b|c and 1\t2\tfake.go",
		"",
		"4\t0\treal.go",
	)

	var got Commit
	if err := parse(strings.NewReader(in), func(c Commit) error { got = c; return nil }); err != nil {
		t.Fatal(err)
	}

	if want := "fix a|b|c and 1\t2\tfake.go"; got.Subject != want {
		t.Errorf("subject %q, want %q", got.Subject, want)
	}
	if len(got.Changes) != 1 || got.Changes[0].Path != "real.go" {
		t.Errorf("changes %+v, want only real.go", got.Changes)
	}
}

func TestParseBinaryAndEmptyCommit(t *testing.T) {
	in := log(
		"\x01abc|Ada|ada@example.com|1700000000|2023-11-15T01:43:20+03:30|add a logo",
		"",
		"-\t-\tdocs/logo.png",
		"",
		"\x01def|Ada|ada@example.com|1700000001|2023-11-15T01:43:21+03:30|merge branch 'main'",
		"",
	)

	var got []Commit
	if err := parse(strings.NewReader(in), func(c Commit) error {
		got = append(got, c)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(got) != 2 {
		t.Fatalf("got %d commits, want 2", len(got))
	}
	if c := got[0].Changes[0]; !c.Binary || c.Added != 0 {
		t.Errorf("binary change parsed as %+v", c)
	}
	// A merge carries no numstat, and a commit with no changes must still
	// arrive: it counts towards the repo's activity.
	if len(got[1].Changes) != 0 {
		t.Errorf("merge commit got %d changes, want 0", len(got[1].Changes))
	}
}

func TestSplitRename(t *testing.T) {
	cases := []struct {
		in               string
		oldPath, newPath string
	}{
		{"internal/store/matches.go", "internal/store/matches.go", "internal/store/matches.go"},
		{"internal/{store => data}/matches.go", "internal/store/matches.go", "internal/data/matches.go"},
		{"old.go => new.go", "old.go", "new.go"},
		{"{old.go => new.go}", "old.go", "new.go"},
		// One side of the brace empty: a file moved into or out of a directory.
		// Joining the halves naively leaves a doubled slash here.
		{"internal/{ => store}/matches.go", "internal/matches.go", "internal/store/matches.go"},
		{"{ => internal}/main.go", "main.go", "internal/main.go"},
		// A brace that is part of the name, not a rename.
		{"web/{id}.html", "web/{id}.html", "web/{id}.html"},
	}

	for _, c := range cases {
		gotOld, gotNew := SplitRename(c.in)
		if gotOld != c.oldPath || gotNew != c.newPath {
			t.Errorf("SplitRename(%q) = %q, %q; want %q, %q",
				c.in, gotOld, gotNew, c.oldPath, c.newPath)
		}
	}
}

// Returning an error from the callback stops the walk, and the error comes back
// unchanged rather than wrapped in something about git.
func TestParseStopsOnError(t *testing.T) {
	in := log(
		"\x01a|Ada|ada@example.com|1700000000|2023-11-15T01:43:20+03:30|one",
		"",
		"1\t0\ta.go",
		"",
		"\x01b|Ada|ada@example.com|1700000001|2023-11-15T01:43:21+03:30|two",
		"",
		"1\t0\tb.go",
	)

	stop := errStop{}
	n := 0
	err := parse(strings.NewReader(in), func(Commit) error {
		n++
		return stop
	})

	if err != stop {
		t.Errorf("got error %v, want it passed through unchanged", err)
	}
	if n != 1 {
		t.Errorf("callback ran %d times, want 1", n)
	}
}

type errStop struct{}

func (errStop) Error() string { return "stop" }

// %at is an instant and says nothing about the hour its author saw. Two people
// commit at the same moment and one of them is up at two in the morning; the
// offset is the only thing that can tell them apart, so it has to survive the
// parse.
func TestParseKeepsTheAuthorsOwnHour(t *testing.T) {
	// 1700000000 is 2023-11-14T22:13:20Z, which is the next day at +03:30.
	lines := []string{
		"\x01abc|Ada|ada@example.com|1700000000|2023-11-15T01:43:20+03:30|late",
		"1\t0\ta.go",
	}

	var got []Commit
	if err := parse(strings.NewReader(log(lines...)), func(c Commit) error {
		got = append(got, c)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d commits, want 1", len(got))
	}

	if h := got[0].When.Hour(); h != 22 {
		t.Errorf("When is hour %d, want 22: When stays UTC", h)
	}
	if h := got[0].Local.Hour(); h != 1 {
		t.Errorf("Local is hour %d, want 1: the author committed after their own midnight", h)
	}
	if !got[0].Local.Equal(got[0].When) {
		t.Error("Local and When are different instants; only the zone should differ")
	}
}

// A log with no %aI in it — an older git, or a fixture written by hand — still
// parses. It just has no hour of its own.
func TestParseWithoutAnOffsetFallsBackToUTC(t *testing.T) {
	lines := []string{
		"\x01abc|Ada|ada@example.com|1700000000|not a date|subject",
		"1\t0\ta.go",
	}

	var got []Commit
	if err := parse(strings.NewReader(log(lines...)), func(c Commit) error {
		got = append(got, c)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || !got[0].Local.Equal(got[0].When) {
		t.Fatalf("a commit with an unreadable date should borrow UTC's hour")
	}
}
