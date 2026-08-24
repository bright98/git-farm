# git-farm

A repository drawn as a pixel farm, in the terminal and in your README.

<picture>
  <source media="(prefers-color-scheme: dark)" srcset=".github/farm-dark.svg">
  <img alt="git-farm's own repository drawn as a farm: fields for its directories, fenced where tests were found" src=".github/farm.svg">
</picture>

*This repository, drawn by itself. The small figure is whoever committed last,
standing in the directory they committed to, and they walk.*

```
git farm
```

No server. No account. No network. It reads `git log` in the folder you are
standing in and draws a picture.

A directory is a field, a file is a plant. It looks like a toy, and it is really
a code health map: three of the things it draws are true and useful, and the
rest is decoration.

```
   ┌─ internal/games/ ─────────────────┐  ┌─ internal/store/ ───┐  ┌─ ./          ──┐
   │  ▄█▄                              │  │                     │  │                │
   │ ▀▀█▀▀   ▄█▄    ▄█▄    ▄█▄         │  │ ▀▄█▄▀  ▀▄█▄▀        │  │  ▄█▄   ▀▄█▄▀   │
   │   █      █      █      █          │  │  ▀▄▀    ▀▄▀         │      █     ▀▄▀
   │   ▄      ▄      ▄    ▄███▄        │  │ ▄ ▄ ▄    ▄          │      ▄      ▄
   │  ▀█▀    ▀█▀    ▀█▀     █          │  │  █▀█    ▀█▀         │     ▀█▀    ▀█▀
   │   ▀      ▀      ▀      ▀          │  │   ▀     ▄█▄         │      ▀      ▀
   │    ▄                              │  │  ▄█▄   ▀▀█▀▀        │     ▄█▄    ▄█▄
   │   ▀█▀                             │  │   █      █          │      █      █
   │   ███                             │  │   ▄      ▄          │      ▄
   │   █ █                             │  │  ▀█▀    ▀█▀         │  │  ▀█▀           │
   └───────────────────────────────────┘  └─────────────────────┘  └──            ──┘
```

That is ssh-games with the colour taken out, which is how it looks piped to a
file. `internal/store/` has weeds in it; `./` is a directory nothing can be
claimed about; the little figure in the corner of `internal/games/` is whoever
committed last, standing where they committed.

| On the farm | In the repo |
|---|---|
| a fenced field | the directory has tests |
| a broken fence | no test files were found |
| corners only, no fence | no rule applies here, so no claim is made |
| a green upright plant | a file changed recently |
| a tall plant | a big file |
| **weeds** | **churn: many authors, many commits, same file** |
| a grey bent plant | untouched for years |
| a flat mark at ground level | a file that was deleted |
| the farmer | the author of the newest commit, standing in that field |

## Status

**Phases 0 to 2 of [the plan](git-farm-plan.md) are done: it reads a repository,
draws it in the terminal, and writes it as an SVG.** Still to come: the GitHub
Action that keeps the picture up to date (phase 3), the TUI (phase 4) and the
time-lapse (phase 5).

```sh
go build ./cmd/git-farm
./git-farm                              # this repo, in this window
./git-farm --theme full                 # the painted version, with sky and soil
./git-farm --out farm.svg --theme both  # farm.svg and farm-dark.svg, for a README
./git-farm --list                       # the same thing as a table
./git-farm --json                       # the parsed repo, which the picture is drawn from
./git-farm ~/src/some-repo
```

## What it measures, and how much to trust it

Everything comes from one command — `git log --reverse --numstat` — plus the
list of files at HEAD. No GitHub API: file churn over 5,000 commits is one local
command or several thousand rate-limited requests, and the local one also works
on GitLab, on a folder with no remote, and inside a CI checkout that has the
history on disk already.

**Ranks, never fixed numbers.** "More than 40 commits is a hotspot" makes a
small repo look immaculate and a large one look doomed. Everything is a
percentile inside the repo it came from, so the picture always has some weeds
and some calm — with one guard: when a distribution is flat, nothing is a peak.
A repo where every file has been committed once by one person grows no weeds.

| Mark | Rule |
|---|---|
| big (tall plant) | lines in the top 10% of files in this repo |
| weed | `commits × distinct authors` in the top 10% |
| dry | no commit has touched it for more than a year |
| deleted | in the history, but not in the HEAD tree |

| Field | Rule, checked in this order |
|---|---|
| `dead` | nothing alive under it touched for more than a year |
| `hotspot` | its median file churn is in the repo's top 20% |
| `untested` | no test files found, and the rules apply here |
| `healthy` | everything else |

**Plant height comes from line count, which is a weak measure of importance.** A
40-line file can matter more than a 4,000-line one. Churn, silence and tests are
the claims worth reading; height is decoration.

**Tests have three states, not two.** The fence is the one claim that can be
wrong in public, on somebody else's README. A Rust file with `#[cfg(test)] mod
tests` is tested and has no test file, so its contents are read rather than
guessed at; a directory of SQL migrations gets no verdict at all, because there
is no code there to test; a directory of documents with one script in it is a
documentation directory, not an untested package. When no rule applies, the
field is drawn with crop marks at its corners instead of a fence — the extent is
still clear, and nothing has been claimed.

`tests: [...]` in the config adds globs for a layout none of the built-in rules
knows about.

**Time is measured back from the newest commit, not from today.** `--since 5y`
means five years before HEAD, and "untouched for a year" means a year before
HEAD. Two runs a week apart over the same commit therefore produce the same
farm, which is what lets the eventual Action push nothing when nothing changed.

## How the picture is put together

**One field per directory, laid out as a treemap** — fields are rectangles
already. The layout is computed once, from the state at HEAD, never per frame:
recomputing it would make every new file re-flow the whole farm.

**A field's area follows the square root of its file count.** Homebrew is the
argument for this. `Library/Homebrew/` holds 2,845 of its 5,600 files and every
other directory is under a hundred, so area in proportion to the count gives
that one field 93% of the farm and leaves the rest as slivers too small to draw
— a picture of one directory, from a repository that has forty. A field never
draws all its files anyway; what it needs room for is the *mix*, and that grows
much more slowly than the count.

**Directories that will not fit are gathered into one field called `other/`,
never dropped.** How many fields fit is an estimate, so it is checked: if the
treemap comes back with a field too small to draw, one more directory is
gathered up and the whole thing is laid out again.

**A field with more files than squares shows a sample that keeps the
proportions** — with one rule on top: a kind that exists at all gets at least
one square. The picture will over-state two weeds among three hundred files, and
that is the intended direction to be wrong in. A farm that hides the weeds is no
use.

**Two themes.** `quiet` is the default in a terminal: it leaves the sky, the
grass and the soil unpainted, so the session shows through and the farm sits in
the terminal instead of covering it. `full` paints a whole little world, and is
the one for the README image, where there is no session to sit in.

**Terminals that are not truecolor.** The palette is seven colours, so the
mapping is seven lookups done once: the 216-colour cube plus 24 greys at 256
colours, the nearest ANSI name at 16 — which lets the user's own theme pick the
shade. `NO_COLOR`, a `TERM` of `dumb`, or stdout that is not a terminal all mean
no escape codes at all, and the picture falls back to one bit. It still reads,
because the plants are held apart by silhouette and ink density before colour:
live is 6 pixels and upright, churn is 9 and sprawling, dead is 4 and bent over.
All three rules are tested.

## The SVG

`--out farm.svg` writes the picture instead of printing it. It is the same farm,
drawn for a medium that has things a terminal does not.

**One `<path>` per colour.** Each row is cut into runs of one colour, runs that
sit directly above each other are merged into rectangles, and every rectangle of
one colour goes into a single path. A 120×72 farm is 8,640 pixels and comes out
as about seventy elements — 7 KB for the quiet theme, 27 KB for the painted one.

**Real text for the names, real strokes for the fences.** In a terminal, a
box-drawing glyph is the only thing thinner than a pixel; in SVG a stroke can be
any width and dashes are real dashes, so the three fence states are a solid
stroke, a dashed one, and crop marks at the corners. The names are `<text>`, set
in the gap the fence leaves for them, and they stay sharp at any size. The
wooden fence of the `full` theme is not replaced — that one is pixel art, and it
stays in the pixels.

**Fuller plants, and a taller canvas to hold them.** The terminal's sprites are
four pixels tall because they sit on top of a live session and must not cover
it. A file has nothing behind it, so it gets a leafier set — which obeys the
same tested rules about silhouette and ink density, with more room to obey them
in. Which is also why the file is 120×50 where a terminal is 120×36: bigger
plants need taller fields, and a field too short to plant is one the layout
gathers away, so a file at a terminal's shape quietly draws half the
directories.

**Two files, because a file cannot ask.** `--theme both` writes `farm.svg` and
`farm-dark.svg` with palettes tuned for a white and a dark page; neither paints
a background, so the README shows through. The dark one is the farm after dark:
a crescent moon where the sun was, and a lantern lit beside the farmer. A reader
on a dark page is not literally awake at night, but those are the two things the
night has, and they are the two that belong on a dark page — and drawing the
pair as day and night tells them apart at a glance rather than only in palette. `--theme full` paints its own little
world and needs only one file.

The quiet theme is built to sit on a dark terminal, which is the right default
for a session and the wrong one for a file: a file has no session behind it, and
a README page is light until its reader says otherwise. So `--out` with no
`--theme` writes the light palette, and `--theme quiet` still writes the dark one
for a page that wants it.

**The farmer walks, and nothing else moves.** CSS keyframes over a static
background — never `<script>`, which GitHub strips. The two frames are swapped
with `fill-opacity` rather than `opacity`: a README embeds the file with `<img>`,
which is rendered without a compositor, and the properties normally handed to
the compositor are the ones at risk of being dropped. `prefers-reduced-motion`
stops it entirely.

**A tooltip on every field, and on the farmer.** An invisible patch over each
field carries a `<title>`: the directory, the three claims the legend makes
about it, and the counts they were made from — `internal/store/ — a hotspot,
tests found — 22 files, 14 churned, 2 big`. The farmer's walks with them and
names the person. Nothing else is named, because nothing else is a real thing: a
field with three hundred files gets twenty squares, and the plants in it are a
sample that keeps the proportions rather than twenty particular files, so
pointing at one and giving it a name would be inventing a fact.

None of it shows on a GitHub README, which embeds the file with `<img>` — that
makes it an image document, a picture with no DOM to hover. The tooltips are for
the reader who clicks through to the file itself, and for any page that puts the
SVG inline. Naming a plant properly is phase 4's problem, where a cursor lands
on one.

**The same bytes every time.** Colours are sorted rather than taken from a map,
nothing is timestamped, and the file is not rewritten when it has not changed —
so a re-run over an unchanged repository is genuinely a no-op, not one that only
looks like one to git.

## Refusals

Four cases produce a farm that is quietly wrong, so they produce an error
instead:

- **not a git repository**
- **a repository with no commits**
- **a shallow clone** — somebody forgot `fetch-depth: 0` and got one commit of
  history, which makes every file look new and every directory look like a
  hotspot. The message says how to fix it.
- **a window under 60×18** — a farm squeezed into less is not a smaller picture,
  it is an unreadable one.

## Flags

| Flag | Default | |
|---|---|---|
| `--theme` | `quiet` | `quiet`, `full`, or `both` (an SVG in light and dark) |
| `--out` | | write an SVG here instead of drawing in the terminal |
| `--scale` | `6` | SVG units per pixel of the farm |
| `--color` | `auto` | `auto`, `full`, `256`, `16`, `none` |
| `--no-color` | | shorthand for `--color none`; `NO_COLOR` does the same |
| `--names` | `true` | directory names on the fields, and who committed last |
| `--night` | | draw it at night |
| `--width`, `--height` | fit the window | |
| `--list` | | the fields as a table instead of a picture |
| `--json` | | the parsed repo; the debugging escape hatch |
| `--since` | `5y` | ignore history older than this, measured back from HEAD |
| `--max-commits` | `0` | 0 means no limit |
| `--depth` | `2` | how many path segments a field name keeps |
| `--config` | `.git-farm.json` | thresholds, overlaid on the defaults |
| `--no-cache` | | re-read the history even if a cached answer exists |

Every threshold lives in one struct with defaults, and a config file overrides
any of them without restating the rest:

```json
{
  "quiet": "18m",
  "churn": 0.05,
  "depth": 3,
  "tests": ["qa/", "*_check.rb"]
}
```

## Speed

The parsed repository is cached inside `.git/git-farm/`, keyed on the HEAD SHA
and on every option that changes what gets read. Nothing is ever committed, and
the clone takes the cache with it when it is deleted.

Homebrew's own repository — 22,403 commits and 5,582 files inside the default
five-year window — takes about 11 seconds cold and 0.06 seconds warm.

The cache deliberately does not cover the working tree, which is read for line
counts and inline tests. An uncommitted edit will not invalidate it;
`--no-cache` is the way past that.

## Safety

Nothing leaves the machine. git-farm makes no network calls at all — it runs
`git log`, reads files in the checkout, and writes a picture. That is checkable
with one grep.

The picture shows directory paths, and the name of whoever committed last.
`--names=false` removes both.

## Layout

| | |
|---|---|
| `cmd/git-farm/` | the command; named so that `git farm` works from the PATH |
| `internal/gitlog/` | runs `git log --numstat` and stream-parses it, including renames |
| `internal/repo/` | files rolled into directories, and the four kinds decided |
| `internal/cache/` | the parsed repo, keyed on HEAD |
| `farm/` | the canvas, the themes, the treemap, the drawing and the SVG |

The picture is tested as letters, not as colours: `farm/dump_test.go` prints the
canvas one character per role, which is what makes a layout bug visible in a
test log. It caught three of them while this was being written — a field
silently dropped from the treemap, a farmer standing on top of the plants, and a
sun with one ray outside the sky.

Four more came from running it over repositories nobody wrote it against, which
is the only way to find them. Every one was silent: a picture came out, and it
was wrong.

- **The farmer went missing from the file.** The file themes draw a farmer half
  again as tall as the terminal's, so a field with room for a person on screen
  could have none in the SVG — and the farmer is the only thing in the file that
  moves, so the animation vanished with them. A short field now takes the small
  farmer rather than none.
- **A release commit is an empty commit.** bubbletea's HEAD is `v2.0.9`, which
  changes no file. Read literally it left the farm with nobody in it, while
  `--list` claimed the newest commit was in `./`. The farmer now belongs to the
  newest commit that *touched* something; the clock still comes from the newest
  commit.
- **`--out` wrote the dark palette by default**, because the quiet theme is
  built for a dark terminal — so a plain `git farm --out farm.svg` gave a light
  README pale green on white.
- **Fields came out empty.** The smallest field worth drawing was one number for
  every theme, measured against the quiet terminal's four-pixel plants. A file
  grows plants half again as tall, so a directory with six files in it was drawn
  as a fenced rectangle with a name on it and bare soil inside. The minimum is
  now the theme's own arithmetic, and the file is given a canvas with room for
  what it grows.

What is not tested here is the last step of all: whether the animation runs
inside a GitHub README. It runs when the file is opened in a browser, and the
mechanism is the one the contribution-snake action uses, but headless Chrome
will not advance animations in an image document at all — so that claim waits
for phase 3, which is what puts the file in front of GitHub.

## License

MIT.
