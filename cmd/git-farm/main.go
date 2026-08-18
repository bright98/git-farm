// Command git-farm draws a repository as a pixel farm.
//
// It is named so that `git farm` works: git runs any git-farm on the PATH as a
// subcommand, for free.
//
// Nothing here touches the network. It reads the history that is already on
// disk and writes a picture — which is the whole security story, and one people
// can check with a grep.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/haleh/git-farm/internal/cache"
	"github.com/haleh/git-farm/internal/gitlog"
	"github.com/haleh/git-farm/internal/repo"
)

// version is stamped by the release build; the default is what a `go build`
// from a working copy gets.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "git farm: "+err.Error())
		os.Exit(1)
	}
}

func run() error {
	var (
		asJSON     = flag.Bool("json", false, "print the parsed repository as JSON and stop")
		since      = flag.String("since", "5y", "ignore history older than this, measured back from the newest commit")
		maxCommits = flag.Int("max-commits", 0, "read at most this many commits; 0 means all of them")
		depth      = flag.Int("depth", 0, "how many path segments a field name keeps (default 2)")
		configPath = flag.String("config", "", "a JSON config file of thresholds (default .git-farm.json in the repo root)")
		noCache    = flag.Bool("no-cache", false, "re-read the history even if a cached answer exists")
		showVer    = flag.Bool("version", false, "print the version and exit")
	)

	flag.Usage = usage
	flag.Parse()

	if *showVer {
		fmt.Println("git-farm " + version)
		return nil
	}

	dir := "."
	if flag.NArg() > 0 {
		dir = flag.Arg(0)
	}

	span, err := repo.ParseSpan(*since)
	if err != nil {
		return err
	}

	// Inspect first: it is what turns "not a git repo", "no commits" and
	// "shallow clone" into one clear line instead of a wrong picture.
	info, err := gitlog.Inspect(dir)
	if err != nil {
		if errors.Is(err, gitlog.ErrNotRepo) || errors.Is(err, gitlog.ErrEmpty) || errors.Is(err, gitlog.ErrShallow) {
			return err
		}
		return fmt.Errorf("reading %s: %w", dir, err)
	}

	cfg, err := loadConfig(*configPath, info.Root)
	if err != nil {
		return err
	}
	if *depth > 0 {
		cfg.Depth = *depth
	}

	opts := repo.Options{Since: span, MaxCommits: *maxCommits}
	r, err := load(info, cfg, opts, *noCache)
	if err != nil {
		return err
	}

	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(r)
	}

	// The picture is the next phase. Until then, print what the picture will be
	// drawn from — which is also the answer to every "why is this field grey".
	fmt.Print(summary(r))
	return nil
}

// load returns the parsed repository, from the cache when the cache still
// applies.
//
// The key covers the HEAD SHA and every option that changes what gets read. It
// deliberately does not cover the working tree, which is read for line counts
// and for inline tests: an uncommitted edit will not invalidate the cache, and
// --no-cache is the way past that.
func load(info gitlog.Info, cfg repo.Config, opts repo.Options, noCache bool) (*repo.Repo, error) {
	fingerprint, _ := json.Marshal(cfg)
	key := cache.Key(info.Head,
		opts.Since.String(),
		strconv.Itoa(opts.MaxCommits),
		string(fingerprint),
	)

	if !noCache && info.GitDir != "" {
		var cached repo.Repo
		if cache.Load(info.GitDir, key, &cached) {
			return &cached, nil
		}
	}

	r, err := repo.Build(info.Root, cfg, opts)
	if err != nil {
		return nil, err
	}

	if info.GitDir != "" {
		// A cache that cannot be written is not worth failing a run over.
		_ = cache.Save(info.GitDir, key, r)
	}
	return r, nil
}

// loadConfig looks for the file the user named, and otherwise for one in the
// repository root.
func loadConfig(path, root string) (repo.Config, error) {
	if path == "" {
		return repo.LoadConfig(root + "/.git-farm.json")
	}
	cfg, err := repo.LoadConfig(path)
	if err != nil {
		return cfg, err
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return cfg, fmt.Errorf("config %s: %w", path, statErr)
	}
	return cfg, nil
}

// summary is the parsed repo in words: one line per field, worst first.
func summary(r *repo.Repo) string {
	var b strings.Builder

	fmt.Fprintf(&b, "%s\n", r.Root)
	fmt.Fprintf(&b, "%s commits, %s, %s, %s\n",
		comma(r.Commits), plural(r.Authors, "author"), plural(len(r.Files), "file"),
		span(r.First, r.Last))
	if r.LastAuthor != "" {
		fmt.Fprintf(&b, "newest commit by %s in %s\n", r.LastAuthor, or(r.LastDir, "./"))
	}
	b.WriteByte('\n')

	// Worst first: the point of the tool is that the bad corner finds you.
	dirs := append([]*repo.Dir(nil), r.Dirs...)
	rank := map[repo.Kind]int{repo.Hotspot: 0, repo.Untested: 1, repo.Dead: 2, repo.Healthy: 3}
	sort.SliceStable(dirs, func(i, j int) bool {
		if rank[dirs[i].Kind] != rank[dirs[j].Kind] {
			return rank[dirs[i].Kind] < rank[dirs[j].Kind]
		}
		return dirs[i].Churn > dirs[j].Churn
	})

	width := 4
	for _, d := range dirs {
		width = max(width, len(d.Path))
	}

	fmt.Fprintf(&b, "%-*s  %-8s  %-7s  %5s  %5s  %5s  %5s\n",
		width, "field", "kind", "tests", "files", "weeds", "big", "dry")
	for _, d := range dirs {
		fmt.Fprintf(&b, "%-*s  %-8s  %-7s  %5d  %5d  %5d  %5d\n",
			width, d.Path, d.Kind, d.Tests, d.Files, d.Weeds, d.Big, d.Dry)
	}

	b.WriteString("\ntests: found = test files or inline tests, none = the rules apply and found nothing,\n")
	b.WriteString("       unknown = no rule here that can be trusted, so no claim is made\n")
	return b.String()
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return comma(n) + " " + noun + "s"
}

func span(first, last time.Time) string {
	if first.IsZero() || last.IsZero() {
		return "no history in the window"
	}
	days := int(last.Sub(first).Hours() / 24)
	switch {
	case days < 1:
		return "less than a day of history"
	case days > 730:
		return fmt.Sprintf("%.1f years of history", float64(days)/365)
	case days > 60:
		return fmt.Sprintf("%d months of history", days/30)
	default:
		return fmt.Sprintf("%d days of history", days)
	}
}

func or(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

func comma(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func usage() {
	fmt.Fprint(os.Stderr, `git farm — a repository drawn as a pixel farm

usage: git farm [flags] [path]

Reads the git history in the folder you are standing in and reports what it
found. Nothing leaves the machine.

flags:
`)
	flag.PrintDefaults()
}
