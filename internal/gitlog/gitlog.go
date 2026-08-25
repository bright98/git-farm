// Package gitlog turns a checked-out repository into a stream of commits.
//
// Everything git-farm knows about a repo comes from one command:
//
//	git log --reverse --numstat --pretty=format:'%H|%an|%ae|%at|%s'
//
// Per commit: author, email, timestamp, subject, and then one line per file
// with lines added, lines deleted and the path. No network, no GitHub API, no
// token — the history is already on disk, and reading it locally costs one
// process instead of thousands of requests.
package gitlog

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Change is one file touched by one commit.
type Change struct {
	Path    string // the path after this commit; the new name on a rename
	OldPath string // the previous name, when this commit renamed the file
	Added   int
	Deleted int
	Binary  bool // git prints "-" instead of counts for binary files
}

// Commit is one commit and every file it touched. Merge commits carry no
// changes: git prints no numstat for them, which is what keeps a merge from
// counting every file in the branch twice.
type Commit struct {
	Hash    string
	Author  string
	Email   string
	When    time.Time // always UTC
	Subject string

	// Local is the same moment in the author's own timezone, which is the only
	// way to say what hour they committed at. Two people commit at the same
	// instant and one of them is up after midnight; %at cannot tell them apart
	// because it has already thrown the offset away.
	Local   time.Time
	Changes []Change
}

// Info is what git can say about a repository before its history is read.
type Info struct {
	Root   string    // the top level of the working tree
	GitDir string    // the .git directory, where the cache lives
	Head   string    // the full SHA at HEAD; the cache key
	When   time.Time // when the HEAD commit was made, UTC
}

// The three ways a repository can be unreadable. Each one is a one-line refusal
// with the fix in it, because each one otherwise produces a farm that is
// quietly wrong.
var (
	ErrNotRepo = errors.New("not a git repository: git-farm draws the history of the repo you are standing in")
	ErrEmpty   = errors.New("this repository has no commits yet, so there is no farm to draw")
	ErrShallow = errors.New("shallow clone: git-farm needs the whole history.\n" +
		"  In a GitHub Action, add `fetch-depth: 0` to actions/checkout.\n" +
		"  Locally, run `git fetch --unshallow`.")
)

// Options narrow how much history is read. Both are worth having on a repo with
// 50,000 commits, where the oldest years say nothing about the code as it is
// today.
type Options struct {
	Since      string // a git date such as "5y" or "2021-01-01"; empty means all of it
	MaxCommits int    // 0 means no limit
}

// Inspect answers the three questions that decide whether there is anything to
// draw, before any history is read.
func Inspect(dir string) (Info, error) {
	root, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return Info{}, ErrNotRepo
	}

	// A shallow clone has a history that stops abruptly, so every file looks
	// new and every directory looks like a hotspot. Refusing is the only honest
	// answer: the farm would be wrong without looking wrong.
	if shallow, err := run(dir, "rev-parse", "--is-shallow-repository"); err == nil && shallow == "true" {
		return Info{}, ErrShallow
	}

	head, err := run(dir, "rev-parse", "--verify", "HEAD")
	if err != nil {
		return Info{}, ErrEmpty
	}

	when, err := run(dir, "log", "-1", "--pretty=%at")
	if err != nil {
		return Info{}, ErrEmpty
	}
	secs, err := strconv.ParseInt(when, 10, 64)
	if err != nil {
		return Info{}, fmt.Errorf("git printed an unreadable commit time %q", when)
	}

	gitDir, _ := run(dir, "rev-parse", "--absolute-git-dir")

	return Info{
		Root:   root,
		GitDir: gitDir,
		Head:   head,
		When:   time.Unix(secs, 0).UTC(),
	}, nil
}

// HeadFiles is the set of paths that exist at HEAD.
//
// It is how a deleted file is recognised: the log says a file was once touched,
// and this says whether it is still there. Nothing else in the history tells
// you reliably, because a file can be deleted and added back.
func HeadFiles(dir string) (map[string]bool, error) {
	cmd := exec.Command("git", "-c", "core.quotepath=false", "ls-tree", "-r", "-z", "--name-only", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git ls-tree: %w", err)
	}

	files := make(map[string]bool)
	for _, p := range bytes.Split(out, []byte{0}) {
		if len(p) > 0 {
			files[string(p)] = true
		}
	}
	return files, nil
}

// The header is prefixed with a control character so that a commit line can
// never be confused with a numstat line — a subject can contain anything at
// all, including something that looks exactly like a file path.
const headerMark = '\x01'

// %aI carries the author's timezone offset, which %at does not. It goes before
// the subject, because a subject is the one field allowed to contain a pipe.
const logFormat = "%x01%H|%an|%ae|%at|%aI|%s"

// Walk streams the history through fn, oldest commit first.
//
// Streaming rather than collecting: a repo with 50,000 commits and 20 files per
// commit is a million change records, and none of them are needed twice.
// Returning an error from fn stops the walk and is returned unchanged.
func Walk(dir string, opts Options, fn func(Commit) error) error {
	args := []string{"-c", "core.quotepath=false", "log", "--reverse", "--numstat", "-M", "--pretty=format:" + logFormat}
	if opts.Since != "" {
		args = append(args, "--since="+opts.Since)
	}
	if opts.MaxCommits > 0 {
		// --max-count keeps the newest commits, which --reverse then hands over
		// oldest first. Truncating the far end of history is the point.
		args = append(args, "--max-count="+strconv.Itoa(opts.MaxCommits))
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("running git log: %w", err)
	}

	parseErr := parse(stdout, fn)

	// Drain whatever is left, or git blocks on a full pipe forever when fn
	// stopped early.
	_, _ = io.Copy(io.Discard, stdout)
	waitErr := cmd.Wait()

	if parseErr != nil {
		return parseErr
	}
	if waitErr != nil {
		return fmt.Errorf("git log: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// parse reads the log stream and calls fn once per commit.
func parse(r io.Reader, fn func(Commit) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024) // a subject line can be long

	var cur Commit
	var have bool

	flush := func() error {
		if !have {
			return nil
		}
		have = false
		return fn(cur)
	}

	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}

		if line[0] == headerMark {
			if err := flush(); err != nil {
				return err
			}
			c, ok := parseHeader(line[1:])
			if !ok {
				continue // a malformed header: skip the commit rather than guess
			}
			cur, have = c, true
			continue
		}

		if !have {
			continue // numstat with no commit above it: nothing to attach it to
		}
		if ch, ok := parseNumstat(line); ok {
			cur.Changes = append(cur.Changes, ch)
		}
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("reading git log: %w", err)
	}
	return flush()
}

// parseHeader reads "hash|author|email|unix-time|subject". The subject is last
// precisely so that a "|" in it cannot break the split.
func parseHeader(s string) (Commit, bool) {
	f := strings.SplitN(s, "|", 6)
	if len(f) < 6 {
		return Commit{}, false
	}
	secs, err := strconv.ParseInt(f[3], 10, 64)
	if err != nil {
		return Commit{}, false
	}
	when := time.Unix(secs, 0).UTC() // UTC everywhere, or two runs disagree

	// A repository written by a git too old for %aI, or by something that
	// wrote a date it will not admit to, still has a usable moment. It just
	// has no hour of its own, so it borrows UTC's.
	local, err := time.Parse(time.RFC3339, f[4])
	if err != nil {
		local = when
	}

	return Commit{
		Hash:    f[0],
		Author:  f[1],
		Email:   f[2],
		When:    when,
		Local:   local,
		Subject: f[5],
	}, true
}

// parseNumstat reads "added\tdeleted\tpath". Binary files come through as
// "-\t-\tpath" and are counted as touched but sized zero.
func parseNumstat(line string) (Change, bool) {
	f := strings.SplitN(line, "\t", 3)
	if len(f) < 3 {
		return Change{}, false
	}

	ch := Change{Binary: f[0] == "-" || f[1] == "-"}
	if !ch.Binary {
		a, err1 := strconv.Atoi(f[0])
		d, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			return Change{}, false
		}
		ch.Added, ch.Deleted = a, d
	}

	ch.OldPath, ch.Path = SplitRename(f[2])
	if ch.OldPath == ch.Path {
		ch.OldPath = ""
	}
	return ch, true
}

// SplitRename reads the path field of a numstat line, which is either a plain
// path or one of git's two rename spellings:
//
//	internal/{store => data}/matches.go   the common prefix and suffix factored out
//	old.go => new.go                      when there is nothing in common
//	internal/{ => store}/matches.go       one side of the brace empty
//
// The empty-side form is why the halves are joined with path.Clean rather than
// concatenated: it leaves a doubled slash otherwise.
func SplitRename(p string) (oldPath, newPath string) {
	const arrow = " => "

	open := strings.Index(p, "{")
	if open >= 0 {
		close := strings.Index(p[open:], "}")
		if close < 0 {
			return p, p
		}
		close += open

		mid := p[open+1 : close]
		sep := strings.Index(mid, arrow)
		if sep < 0 {
			return p, p // a brace that is part of the name, not a rename
		}

		prefix, suffix := p[:open], p[close+1:]
		return cleanJoin(prefix, mid[:sep], suffix), cleanJoin(prefix, mid[sep+len(arrow):], suffix)
	}

	if sep := strings.Index(p, arrow); sep >= 0 {
		return p[:sep], p[sep+len(arrow):]
	}
	return p, p
}

func cleanJoin(prefix, mid, suffix string) string {
	s := prefix + mid + suffix
	for strings.Contains(s, "//") {
		s = strings.ReplaceAll(s, "//", "/")
	}
	return strings.TrimPrefix(s, "/")
}

func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
