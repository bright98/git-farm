package repo

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/haleh/git-farm/internal/gitlog"
)

// The tests below build real git repositories in a temp directory and run the
// real git binary over them. Parsing can be tested with a string; classification
// cannot, because half of what it gets wrong — renames, deletions, merges, the
// order commits arrive in — is git's behaviour rather than ours.

type testRepo struct {
	t    *testing.T
	dir  string
	base time.Time
}

func newTestRepo(t *testing.T) *testRepo {
	t.Helper()
	dir := t.TempDir()

	r := &testRepo{
		t:   t,
		dir: dir,
		// A fixed base, so "three years ago" is the same three years on every
		// machine and in every year this test is run.
		base: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}
	r.git("init", "-q", "-b", "main")
	r.git("config", "user.name", "Test")
	r.git("config", "user.email", "test@example.com")
	return r
}

func (r *testRepo) git(args ...string) string {
	r.t.Helper()
	return r.gitAt(r.dir, args...)
}

func (r *testRepo) gitAt(dir string, args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// write puts a file in the working tree, creating directories as needed.
func (r *testRepo) write(path, body string) {
	r.t.Helper()
	full := filepath.Join(r.dir, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// commit stages everything and commits it as author, at a moment offset from
// the base time.
func (r *testRepo) commit(author, email string, ago time.Duration, message string) {
	r.t.Helper()
	when := r.base.Add(-ago).Format(time.RFC3339)

	cmd := exec.Command("git", "commit", "-q", "-a", "-m", message,
		"--author", author+" <"+email+">")
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_AUTHOR_DATE="+when, "GIT_COMMITTER_DATE="+when,
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		r.t.Fatalf("git commit: %v\n%s", err, out)
	}
}

func (r *testRepo) add(paths ...string) {
	r.t.Helper()
	r.git(append([]string{"add", "-A", "--"}, paths...)...)
}

const year = 365 * 24 * time.Hour

// build is the whole pipeline over a repository laid out to hit all four kinds.
func build(t *testing.T) (*Repo, map[string]*Dir, map[string]*File) {
	t.Helper()
	r := newTestRepo(t)

	// A quiet, tested package.
	r.write("internal/ui/view.go", strings.Repeat("// a line\n", 40))
	r.write("internal/ui/view_test.go", "package ui\n")
	r.add(".")
	r.commit("Ada", "ada@example.com", 2*year, "add the ui")

	// Code nobody has touched since. This is what dead means: not deleted, not
	// broken — untouched.
	r.write("legacy/old.go", strings.Repeat("// old\n", 30))
	r.write("legacy/older.go", strings.Repeat("// older\n", 10))
	r.add(".")
	r.commit("Ada", "ada@example.com", 2*year, "add the legacy code")

	// A file that will be deleted later, so its plant becomes a flat mark.
	r.write("legacy/doomed.go", "// doomed\n")
	r.add(".")
	r.commit("Ada", "ada@example.com", 2*year, "add a file that will not last")

	// A command with no tests anywhere: a broken fence.
	r.write("cmd/tool/main.go", strings.Repeat("// main\n", 20))
	r.add(".")
	r.commit("Ada", "ada@example.com", 30*24*time.Hour, "add the command")

	// The hotspot: one file, hammered by three people, plus a big one.
	r.write("internal/store/db.go", strings.Repeat("// db\n", 500))
	r.write("internal/store/db_test.go", "package store\n")
	r.add(".")
	r.commit("Ada", "ada@example.com", 60*24*time.Hour, "add the store")

	for i, who := range []string{"Ada", "Bo", "Cy", "Bo", "Cy", "Ada", "Bo"} {
		r.write("internal/store/db.go", strings.Repeat("// db\n", 501+i))
		r.add(".")
		r.commit(who, strings.ToLower(who)+"@example.com",
			time.Duration(50-i)*24*time.Hour, "change the store again")
	}

	// The deletion, and a rename in the same breath.
	if err := os.Remove(filepath.Join(r.dir, "legacy/doomed.go")); err != nil {
		t.Fatal(err)
	}
	r.git("mv", "internal/ui/view.go", "internal/ui/render.go")
	r.add(".")
	r.commit("Ada", "ada@example.com", 24*time.Hour, "delete one file, rename another")

	got, err := Build(r.dir, Defaults(), Options{})
	if err != nil {
		t.Fatal(err)
	}

	dirs := map[string]*Dir{}
	for _, d := range got.Dirs {
		dirs[d.Path] = d
	}
	files := map[string]*File{}
	for _, f := range got.Files {
		files[f.Path] = f
	}
	return got, dirs, files
}

func TestBuildClassifiesDirectories(t *testing.T) {
	got, dirs, _ := build(t)

	if got.Commits != 13 {
		t.Errorf("read %d commits, want 13", got.Commits)
	}
	if got.Authors != 3 {
		t.Errorf("found %d authors, want 3", got.Authors)
	}

	want := map[string]Kind{
		"internal/store/": Hotspot,  // three people, seven commits, one file
		"cmd/tool/":       Untested, // Go, and no _test.go anywhere under it
		"legacy/":         Dead,     // untouched for two years
		"internal/ui/":    Healthy,
	}
	for path, kind := range want {
		d := dirs[path]
		if d == nil {
			t.Errorf("no field for %s; got %v", path, keys(dirs))
			continue
		}
		if d.Kind != kind {
			t.Errorf("%s is %s, want %s (files %d, weeds %d, dry %d, churn %.1f, tests %s)",
				path, d.Kind, kind, d.Files, d.Weeds, d.Dry, d.Churn, d.Tests)
		}
	}
}

func TestBuildFindsTests(t *testing.T) {
	_, dirs, _ := build(t)

	want := map[string]TestState{
		"internal/ui/":    TestsFound,
		"internal/store/": TestsFound,
		"cmd/tool/":       TestsMissing,
	}
	for path, state := range want {
		if got := dirs[path].Tests; got != state {
			t.Errorf("%s tests = %s, want %s", path, got, state)
		}
	}
}

// A rename must carry the file's history with it. Without this, every rename
// looks like a brand new file and a refactor turns the whole repo green.
func TestRenameKeepsHistory(t *testing.T) {
	_, _, files := build(t)

	renamed := files["internal/ui/render.go"]
	if renamed == nil {
		t.Fatalf("the renamed file is missing; got %v", keys(files))
	}
	if _, stillThere := files["internal/ui/view.go"]; stillThere {
		t.Error("the old name is still in the repo: the rename was not followed")
	}
	if renamed.Commits < 2 {
		t.Errorf("the renamed file has %d commits, want the history it had before the rename", renamed.Commits)
	}
	if renamed.First.After(renamed.Last) {
		t.Error("first seen is after last seen")
	}
}

// A file that is gone at HEAD is a flat mark on the soil, not a plant. The log
// alone cannot tell you this — a file can be deleted and added back — so it
// comes from what is actually in the HEAD tree.
func TestDeletedFileIsMarked(t *testing.T) {
	_, dirs, files := build(t)

	doomed := files["legacy/doomed.go"]
	if doomed == nil {
		t.Fatal("the deleted file is missing from the repo entirely")
	}
	if !doomed.Deleted {
		t.Error("the deleted file is not marked deleted")
	}
	if doomed.Big || doomed.Weed {
		t.Error("a deleted file must not also be a tall plant or a weed")
	}
	if dirs["legacy/"].Deleted != 1 {
		t.Errorf("legacy/ counted %d deleted files, want 1", dirs["legacy/"].Deleted)
	}
}

func TestBigAndWeedAreRanked(t *testing.T) {
	_, _, files := build(t)

	if db := files["internal/store/db.go"]; db == nil {
		t.Fatal("internal/store/db.go is missing")
	} else {
		if !db.Big {
			t.Errorf("the 500-line file is not big (lines %d)", db.Lines)
		}
		if !db.Weed {
			t.Errorf("the file changed by three people seven times is not a weed (churn %d)", db.Churn)
		}
		if db.Authors != 3 {
			t.Errorf("db.go has %d authors, want 3", db.Authors)
		}
	}

	if old := files["legacy/older.go"]; old.Big || old.Weed {
		t.Errorf("the small quiet file was marked big=%v weed=%v", old.Big, old.Weed)
	}
	if !files["legacy/old.go"].Dry {
		t.Error("a file untouched for two years is not dry")
	}
}

// Percentiles on a flat distribution are meaningless: if every file has been
// committed exactly once by one person, the "top 10% by churn" is still 10% of
// the files, and the farm sprouts weeds that mean nothing at all.
func TestFlatRepoGrowsNoWeeds(t *testing.T) {
	r := newTestRepo(t)
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go", "e.go"} {
		r.write("pkg/"+name, "package pkg\n")
	}
	r.add(".")
	r.commit("Ada", "ada@example.com", 0, "add everything at once")

	got, err := Build(r.dir, Defaults(), Options{})
	if err != nil {
		t.Fatal(err)
	}

	for _, f := range got.Files {
		if f.Weed {
			t.Errorf("%s is a weed in a repo where every file has the same history", f.Path)
		}
	}
	for _, d := range got.Dirs {
		if d.Kind == Hotspot {
			t.Errorf("%s is a hotspot in a repo with one commit", d.Path)
		}
	}
}

// --since is measured back from the newest commit, not from today. That is what
// makes two runs a week apart over the same HEAD produce the same farm — and
// the Action's push a no-op instead of a new commit every day.
func TestSinceIsMeasuredFromHead(t *testing.T) {
	r := newTestRepo(t)

	r.write("old/ancient.go", "package old\n")
	r.add(".")
	r.commit("Ada", "ada@example.com", 4*year, "the distant past")

	r.write("new/recent.go", "package new\n")
	r.add(".")
	r.commit("Bo", "bo@example.com", 10*24*time.Hour, "the other day")

	got, err := Build(r.dir, Defaults(), Options{Since: Span(year)})
	if err != nil {
		t.Fatal(err)
	}

	if got.Commits != 1 {
		t.Errorf("read %d commits, want only the one inside the window", got.Commits)
	}

	// The old file is outside the window but still exists, so it still stands in
	// its field — as something quiet, not as something new.
	var ancient *File
	for _, f := range got.Files {
		if f.Path == "old/ancient.go" {
			ancient = f
		}
	}
	if ancient == nil {
		t.Fatal("a file older than the window disappeared from the farm")
	}
	if ancient.Commits != 0 || !ancient.Dry {
		t.Errorf("the file outside the window is commits=%d dry=%v, want 0 and dry",
			ancient.Commits, ancient.Dry)
	}
}

// The three refusals. Each of these otherwise produces a farm that is quietly
// wrong, which is worse than no farm at all.
func TestRefusesUnreadableRepositories(t *testing.T) {
	t.Run("not a repository", func(t *testing.T) {
		_, err := Build(t.TempDir(), Defaults(), Options{})
		if !errors.Is(err, gitlog.ErrNotRepo) {
			t.Errorf("got %v, want ErrNotRepo", err)
		}
	})

	t.Run("no commits", func(t *testing.T) {
		r := newTestRepo(t)
		_, err := Build(r.dir, Defaults(), Options{})
		if !errors.Is(err, gitlog.ErrEmpty) {
			t.Errorf("got %v, want ErrEmpty", err)
		}
	})

	// The dangerous one: somebody forgets fetch-depth: 0 in the workflow and
	// gets one commit of history. Every file then looks new, every directory
	// looks like a hotspot, and nothing about the picture says it is wrong.
	t.Run("shallow clone", func(t *testing.T) {
		src := newTestRepo(t)
		src.write("a.go", "package a\n")
		src.add(".")
		src.commit("Ada", "ada@example.com", 2*year, "one")
		src.write("b.go", "package b\n")
		src.add(".")
		src.commit("Ada", "ada@example.com", year, "two")

		clone := filepath.Join(t.TempDir(), "shallow")
		src.gitAt(t.TempDir(), "clone", "-q", "--depth", "1", "file://"+src.dir, clone)

		_, err := Build(clone, Defaults(), Options{})
		if !errors.Is(err, gitlog.ErrShallow) {
			t.Errorf("got %v, want ErrShallow", err)
		}
		if err != nil && !strings.Contains(err.Error(), "fetch-depth: 0") {
			t.Error("the refusal does not say how to fix it")
		}
	})
}

func TestFieldOf(t *testing.T) {
	cases := []struct {
		path  string
		depth int
		want  string
	}{
		{"internal/store/db.go", 2, "internal/store/"},
		{"internal/store/deep/nested/x.go", 2, "internal/store/"},
		{"internal/store/db.go", 1, "internal/"},
		{"main.go", 2, "./"},
		{"docs/x.md", 2, "docs/"},
	}
	for _, c := range cases {
		if got := fieldOf(c.path, c.depth); got != c.want {
			t.Errorf("fieldOf(%q, %d) = %q, want %q", c.path, c.depth, got, c.want)
		}
	}
}

func TestTopThreshold(t *testing.T) {
	cases := []struct {
		name  string
		vals  []int
		share float64
		want  int
	}{
		{"a clear peak", []int{1, 1, 1, 1, 1, 1, 1, 1, 1, 90}, 0.10, 90},
		{"everything equal", []int{5, 5, 5, 5, 5, 5}, 0.10, 0},
		{"nothing at all", nil, 0.10, 0},
		{"one file", []int{7}, 0.10, 0},
	}
	for _, c := range cases {
		if got := topThreshold(c.vals, c.share); got != c.want {
			t.Errorf("%s: topThreshold(%v, %v) = %d, want %d", c.name, c.vals, c.share, got, c.want)
		}
	}
}

func keys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
