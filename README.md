# git-farm

A repository drawn as a pixel farm, in the terminal and in your README.

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

**Phases 0 and 1 of [the plan](git-farm-plan.md) are done: it reads a repository
and draws it in the terminal.** Still to come: the SVG for a README (phase 2),
the GitHub Action that keeps it up to date (phase 3), the TUI (phase 4) and the
time-lapse (phase 5).

```sh
go build ./cmd/git-farm
./git-farm                    # this repo, in this window
./git-farm --theme full       # the painted version, with sky and soil
./git-farm --list             # the same thing as a table
./git-farm --json             # the parsed repo, which is what the picture is drawn from
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
| `--theme` | `quiet` | `quiet` or `full` |
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
| `farm/` | the canvas, the themes, the treemap and the drawing |
| `git-farm-demo/` | where the drawing layer was worked out — a separate module |

The picture is tested as letters, not as colours: `farm/dump_test.go` prints the
canvas one character per role, which is what makes a layout bug visible in a
test log. It caught three of them while this was being written — a field
silently dropped from the treemap, a farmer standing on top of the plants, and a
sun with one ray outside the sky.

## License

MIT.
