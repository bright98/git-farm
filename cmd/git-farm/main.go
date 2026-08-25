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
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/bright98/git-farm/farm"
	"github.com/bright98/git-farm/internal/cache"
	"github.com/bright98/git-farm/internal/gitlog"
	"github.com/bright98/git-farm/internal/repo"
	"github.com/bright98/git-farm/internal/tui"
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
		themeName  = flag.String("theme", "quiet", "quiet, quiet-light, full, or both; --out defaults to quiet-light")
		out        = flag.String("out", "", "write an SVG here instead of drawing in the terminal")
		scale      = flag.Int("scale", 6, "SVG units per pixel of the farm")
		colorName  = flag.String("color", "auto", "auto, full, 256, 16 or none")
		noColor    = flag.Bool("no-color", false, "shorthand for --color none; NO_COLOR does the same")
		names      = flag.Bool("names", true, "write directory names on the fields and say who committed last")
		night      nightMode // registered below, because it is not a plain bool
		width      = flag.Int("width", 0, "terminal columns (default: fit the window)")
		height     = flag.Int("height", 0, "terminal rows (default: fit the window)")
		watch      = flag.Bool("watch", false, "open the farm in a window you can walk around in")
		gifPath    = flag.String("gif", "", "write the history as an animated GIF here")
		frames     = flag.Int("frames", 120, "at most this many frames in the GIF")
		weather    = flag.Bool("weather", false, "let the sky report how busy the repository was")
		asList     = flag.Bool("list", false, "print the fields as a table instead of a picture")
		asJSON     = flag.Bool("json", false, "print the parsed repository as JSON and stop")
		since      = flag.String("since", "5y", "ignore history older than this, measured back from the newest commit")
		maxCommits = flag.Int("max-commits", 0, "read at most this many commits; 0 means all of them")
		depth      = flag.Int("depth", 0, "how many path segments a field name keeps (default 2)")
		configPath = flag.String("config", "", "a JSON config file of thresholds (default .git-farm.json in the repo root)")
		noCache    = flag.Bool("no-cache", false, "re-read the history even if a cached answer exists")
		showVer    = flag.Bool("version", false, "print the version and exit")
	)

	flag.Var(&night, "night", "draw the farm at night: true, false, or auto — auto means the last commit was made after its author's own midnight")

	flag.Usage = usage
	flag.Parse()

	if *showVer {
		fmt.Println("git-farm " + version)
		return nil
	}

	// "both" is not a theme, it is a pair of files: the same farm in the two
	// palettes a README needs, because a file on disk cannot ask the reader
	// whether their page is light or dark.
	theme := farm.ThemeNamed(*themeName)
	if *themeName == "both" {
		if *out == "" {
			return fmt.Errorf("--theme both writes two files, so it needs --out")
		}
		theme = &farm.QuietLight
	}
	if theme == nil {
		return fmt.Errorf("unknown theme %q: try quiet, quiet-light, full, or both", *themeName)
	}

	// The quiet theme is built to sit on a dark terminal, and defaulting to it
	// is right for a session; a file has no session behind it, and a README
	// page is light until its reader says otherwise. So a file nobody named a
	// theme for gets the light one. --theme quiet still writes the dark
	// palette, for a page that wants it.
	if *out != "" && !flagSet("theme") {
		theme = &farm.QuietLight
	}

	// A GIF is pixels and nothing else. The quiet themes draw their fence as a
	// box-drawing character and their names as text, because a terminal has
	// both and neither costs a pixel — so a quiet farm rendered to pixels
	// arrives with no fences at all, which is the one claim the README says
	// must never be wrong in public. The painted theme builds its fence out of
	// pixels, so it survives the trip.
	if *gifPath != "" && !flagSet("theme") {
		theme = &farm.Full
	}
	profile, ok := farm.ParseProfile(*colorName)
	if !ok {
		return fmt.Errorf("unknown colour setting %q: try auto, full, 256, 16 or none", *colorName)
	}
	if *noColor {
		profile = farm.Mono
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
	if *asList {
		fmt.Print(summary(r))
		return nil
	}

	d := drawing{
		theme:   theme,
		profile: profile,
		names:   *names,
		night:   night,
		width:   *width,
		height:  *height,
		scale:   *scale,
		both:    *themeName == "both",
	}
	if *out != "" {
		return writeSVG(r, *out, d)
	}
	if *gifPath != "" {
		return writeGIF(info.Root, cfg, opts, *gifPath, *frames, *weather, d)
	}
	if *watch {
		return watchFarm(r, d)
	}
	return drawFarm(r, d)
}

type drawing struct {
	theme         *farm.Theme
	profile       farm.Profile
	names         bool
	night         nightMode
	width, height int
	scale         int
	both          bool
}

// watchFarm hands the farm to Bubble Tea, which owns the screen until q.
//
// The alt screen, so the session it was run from is still there afterwards —
// the quiet theme is built to sit in a terminal without covering it, and a
// program that scrolls the farm into the scrollback undoes that.
func watchFarm(r *repo.Repo, d drawing) error {
	if len(r.Files) == 0 {
		return fmt.Errorf("no files in the window --since kept: try a longer --since")
	}

	p := tea.NewProgram(
		tui.New(r, d.profile, d.night.at(r)),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}

// drawFarm prints one frame and exits. The version you can walk around in is
// --watch.
func drawFarm(r *repo.Repo, d drawing) error {
	cols, rows := windowSize(d.width, d.height)
	if cols < farm.MinCols || rows < farm.MinRows {
		// Refusing to draw badly. A farm squeezed into a window this size is
		// not a smaller picture, it is an unreadable one.
		return fmt.Errorf("the farm needs a window of at least %d×%d; this one is %d×%d",
			farm.MinCols, farm.MinRows, cols, rows)
	}
	if len(r.Files) == 0 {
		return fmt.Errorf("no files in the window --since kept: try a longer --since")
	}

	scene := farm.FromRepo(r)
	opts := farm.Options{Theme: d.theme, Night: d.night.at(r), Names: d.names}
	picture := scene.Render(cols, rows, opts, d.profile)

	// Drawn is only known once the scene has been laid out against a real
	// window: how many fields fit is a property of the window, not the repo.
	if scene.Drawn() == 0 {
		return fmt.Errorf("nothing to draw: no directory in this repository has enough files for a field")
	}

	fmt.Println(picture)
	fmt.Print(legend(d.theme, d.profile, cols))
	if d.names {
		fmt.Print(caption(r, scene, d.profile))
	}
	return nil
}

// windowSize fits the farm to the terminal, leaving room underneath for the
// legend, and stops it from spreading so wide that it stops being one picture.
func windowSize(width, height int) (cols, rows int) {
	const (
		maxCols  = 120
		reserved = 6 // the blank line, the legend, the caption
	)

	cols, rows = 100, 32
	if w, h, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 20 {
		cols, rows = w, h-reserved
	}
	if cols > maxCols {
		cols = maxCols
	}

	if width > 0 {
		cols = width
	}
	if height > 0 {
		rows = height
	}
	return cols, rows
}

// legend is what makes the picture readable the first time somebody sees it.
func legend(t *farm.Theme, p farm.Profile, cols int) string {
	type item struct {
		glyph, text string
		colour      color.RGBA
	}

	items := []item{
		{"█", "changed lately", t.Colour(farm.RoleLeaf)},
		{"█", "churn: many authors, one file", t.Colour(farm.RoleWeed)},
		{"█", "quiet for a year", t.Colour(farm.RoleDry)},
		{"█", "a big file, and the farmer", t.Colour(farm.RoleShirt)},
		{"─", "test files found", t.Colour(farm.RoleFence)},
		{"┄", "none found", t.Colour(farm.RoleFence)},
		{"┘", "no rule here, so no claim", t.Colour(farm.RoleFence)},
	}

	// Wrapped by the width of the words, not of the string: the escape codes in
	// it are not characters anybody sees.
	const indent, gap = 2, 3
	var out, line strings.Builder
	width := indent

	for _, it := range items {
		w := len([]rune(it.glyph)) + 1 + len([]rune(it.text))
		if line.Len() > 0 {
			if width+gap+w > cols {
				out.WriteString(strings.Repeat(" ", indent) + line.String() + "\n")
				line.Reset()
				width = indent
			} else {
				line.WriteString(strings.Repeat(" ", gap))
				width += gap
			}
		}
		line.WriteString(p.Paint(it.glyph, it.colour) + " " + it.text)
		width += w
	}
	if line.Len() > 0 {
		out.WriteString(strings.Repeat(" ", indent) + line.String() + "\n")
	}
	return out.String()
}

// caption says who is standing in the picture, and where. It is the one place
// the farm names a person, which is why --names turns it off.
func caption(r *repo.Repo, s *farm.Scene, p farm.Profile) string {
	if r.LastAuthor == "" {
		return ""
	}

	where := r.LastDir
	if s.Farmer >= 0 {
		where = s.Fields[s.Farmer].Name
	}
	if where == "" {
		return ""
	}

	return "  " + p.Paint(r.LastAuthor+" committed last, in "+where, farm.Quiet.Colour(farm.RoleLabel)) + "\n"
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
		fmt.Fprintf(&b, "newest change by %s in %s\n", r.LastAuthor, r.LastDir)
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

func comma(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// flagSet reports whether the flag was given on the command line, as opposed to
// left at its default. A default means "you decide", and writing a file and
// drawing in a terminal decide differently.
func flagSet(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// nightMode is the --night flag, which has three settings rather than two.
//
// It is a flag.Value that reports itself as a boolean, so `--night` on its own
// still means yes and `--night=auto` means "ask the repository". Making it a
// plain string flag would have broken the bare form, which is the one people
// type.
type nightMode int

const (
	nightOff nightMode = iota
	nightOn
	nightAuto
)

func (n nightMode) String() string {
	switch n {
	case nightOn:
		return "true"
	case nightAuto:
		return "auto"
	default:
		return "false"
	}
}

func (n *nightMode) Set(v string) error {
	switch strings.ToLower(v) {
	case "true", "1", "yes", "on":
		*n = nightOn
	case "false", "0", "no", "off":
		*n = nightOff
	case "auto":
		*n = nightAuto
	default:
		return fmt.Errorf("night is true, false or auto, not %q", v)
	}
	return nil
}

// IsBoolFlag is what lets `--night` stand on its own.
func (n *nightMode) IsBoolFlag() bool { return true }

// smallHours is when a commit counts as having been made at night: from
// midnight until dawn, by the author's own clock rather than by UTC, because
// "committed after midnight" is a claim about the person and not about
// Greenwich.
const (
	nightFrom = 0 // inclusive
	nightTo   = 6 // exclusive
)

// at resolves the flag against the repository. Only auto asks a question; the
// other two already know the answer.
func (n nightMode) at(r *repo.Repo) bool {
	switch n {
	case nightOn:
		return true
	case nightAuto:
		if r.LastChange.IsZero() {
			return false // no commit changed anything, so nobody was up late
		}
		h := r.LastChange.Hour()
		return h >= nightFrom && h < nightTo
	default:
		return false
	}
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

// writeSVG is the file the README points at.
//
// The size does not come from the window. Nobody is looking at a terminal when
// this runs — it runs on a CI machine with no terminal at all — so the picture
// gets a fixed shape unless one is asked for.
//
// It is taller than a terminal of the same width, because a file grows bigger
// plants: seven pixels between rows against the quiet theme's five, and a
// farmer half again as tall. At a terminal's 120x36 the fields come out too
// short for the crop standing in them, and the layout does the honest thing
// with a field it cannot plant — it draws fewer and bigger ones. bubbletea
// loses half its directories that way. Fifty rows gives the file's plants the
// room the terminal's have.
func writeSVG(r *repo.Repo, path string, d drawing) error {
	cols, rows := 120, 50
	if d.width > 0 {
		cols = d.width
	}
	if d.height > 0 {
		rows = d.height
	}

	scene := farm.FromRepo(r)
	night := d.night.at(r)
	opts := farm.SVGOptions{
		Theme:   d.theme,
		Cols:    cols,
		Rows:    rows,
		Scale:   d.scale,
		Names:   d.names,
		Night:   night,
		Animate: true,
		Title:   svgTitle(r),
	}

	// The second file is the same farm for a dark page. Whether it is also at
	// night depends on which question --night was asked.
	//
	// Left alone, the pair is day and night: a reader on a dark page is not
	// literally awake, but the moon and the lantern are the two things the
	// night has and they are the two that suit a dark page, so the files tell
	// each other apart at a glance rather than only in palette.
	//
	// Set to auto, the sky stops being a decision about the page and becomes a
	// fact about the repository — and a fact cannot be true in one file and
	// false in the other. Both files then say what the last commit's own clock
	// said, and the difference between them goes back to being palette alone.
	type file struct {
		path  string
		theme *farm.Theme
		night bool
	}

	files := []file{{path, d.theme, night}}
	if d.both {
		dark := true
		if d.night == nightAuto {
			dark = night
		}
		files = append(files, file{darkName(path), &farm.Quiet, dark})
	}

	for _, f := range files {
		opts.Theme = f.theme
		opts.Night = f.night
		var buf bytes.Buffer
		if err := scene.WriteSVG(&buf, opts); err != nil {
			return err
		}
		if scene.Drawn() == 0 {
			return fmt.Errorf("nothing to draw: no directory in this repository has enough files for a field")
		}
		changed, err := writeIfChanged(f.path, buf.Bytes())
		if err != nil {
			return err
		}
		if changed {
			fmt.Fprintln(os.Stderr, "wrote "+f.path)
		} else {
			fmt.Fprintln(os.Stderr, "unchanged "+f.path)
		}
	}
	return nil
}

// writeIfChanged leaves the file alone when it already says the same thing.
//
// The output is deterministic, so a repository that has not changed produces
// the same bytes; not touching the file then makes a re-run genuinely a no-op
// rather than one that only looks like one to git.
func writeIfChanged(path string, data []byte) (changed bool, err error) {
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
		return false, nil
	}
	return true, os.WriteFile(path, data, 0o644)
}

// darkName is farm.svg -> farm-dark.svg.
func darkName(path string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "-dark" + ext
}

func svgTitle(r *repo.Repo) string {
	name := filepath.Base(r.Root)
	return fmt.Sprintf("%s drawn as a farm: %s files in %s directories, %s commits",
		name, comma(len(r.Files)), comma(len(r.Dirs)), comma(r.Commits))
}

// writeGIF plays the history into the layout HEAD decided, one frame at a
// time, and writes the lot as a looping image.
//
// The layout is computed once and never again. That is what makes a time-lapse
// readable rather than dizzying: fields keep their place from the first frame
// to the last, so the eye follows the crop growing instead of the ground
// rearranging itself under it.
func writeGIF(root string, cfg repo.Config, opts repo.Options, path string, frames int, weather bool, d drawing) error {
	t, err := repo.History(root, cfg, opts, frames)
	if err != nil {
		return err
	}

	// The last frame is HEAD, and HEAD is what the layout comes from. Building
	// the scene from the final frame rather than from a fresh read keeps the
	// two in step: every field the history ever had is placed, including the
	// ones that are empty by the end.
	last := t.Frames[len(t.Frames)-1]
	scene := farm.FromFrames(t.Frames)

	cols, rows := 120, 40
	if d.width > 0 {
		cols = d.width
	}
	if d.height > 0 {
		rows = d.height
	}

	var buf bytes.Buffer
	err = farm.WriteGIF(&buf, len(t.Frames), farm.GIFOptions{
		Theme: d.theme,
		Cols:  cols,
		Rows:  rows,
		Scale: d.scale / 2,
		Names: d.names,
	}, func(i int, c *farm.Canvas) {
		f := t.Frames[i]
		scene.Play(f)

		where := make([]string, 0, len(f.Farmers))
		for _, w := range f.Farmers {
			where = append(where, w.Dir)
		}
		scene.Draw(c, farm.Options{
			Theme:   d.theme,
			Names:   d.names,
			Night:   nightFor(f, weather, d.night == nightOn),
			Farmers: where,
		})
	})
	if err != nil {
		return err
	}

	changed, err := writeIfChanged(path, buf.Bytes())
	if err != nil {
		return err
	}
	what := "wrote"
	if !changed {
		what = "unchanged"
	}
	fmt.Fprintf(os.Stderr, "%s %s: %d frames, one per %s, %s to %s\n",
		what, path, len(t.Frames), t.Cadence,
		t.Frames[0].When.Format("Jan 2006"), last.When.Format("Jan 2006"))
	return nil
}

// nightFor decides whether a frame is drawn after dark.
//
// Off unless asked for. The plan is firm about this and it is right: a farm
// that goes dark because nobody committed for three months puts a "this project
// is dead" badge on somebody's README without them ever asking for one, and it
// would be the first thing a visitor saw.
func nightFor(f repo.Frame, weather, always bool) bool {
	if always {
		return true
	}
	if !weather {
		return false
	}
	return f.Late || f.Active == 0
}
