# git-farm

A repository drawn as a pixel farm, in the terminal and in your README.

**[See it running → bright98.github.io/git-farm](https://bright98.github.io/git-farm/)** —
the farms on that page are drawn by this repository, with switches for day,
night, light and dark.

<!-- Drawn by this repository's own farm workflow and published to the orphan
     farm branch. The full theme paints its own sky, so it needs one file
     rather than a light and a dark one. See The Action for why it is not
     committed here. -->
<img alt="git-farm's own repository drawn as a farm: fenced fields for its directories, planted with one mark per file"
     src="https://raw.githubusercontent.com/bright98/git-farm/farm/farm.svg">

*This repository, drawn by itself on every push. The small figure is whoever
committed last, standing in the directory they committed to, and they walk. If
that commit was made after their own midnight, the farm is drawn under stars
and they carry a lantern.*

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

`internal/store/` has weeds in it; `./` is a directory nothing can be claimed
about; the little figure in the corner of `internal/games/` is whoever committed
last, standing where they committed.

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

## Quick start

**In a terminal.**

```sh
go install github.com/bright98/git-farm/cmd/git-farm@latest
git farm
```

`git farm` works with a space in it because git runs any `git-farm` on your PATH
as a subcommand, for free. There are prebuilt binaries on the
[releases page](https://github.com/bright98/git-farm/releases) if you would
rather not build one — linux and macOS, Intel and ARM.

```sh
git farm --watch                      # walk around it: a cursor, and what is under it
git farm --gif history.gif            # the whole history, played back
git farm --theme full                 # the painted version, with sky and soil
git farm --out farm.svg --theme both  # farm.svg and farm-dark.svg, for a README
git farm --list                       # the same thing as a table
git farm --json                       # the parsed repo, which the picture is drawn from
git farm ~/src/some-repo              # somebody else's
```

**In your README.** Two files, and the repository grows its own farm on every
push. Add `.github/workflows/farm.yml`:

```yaml
name: farm

on:
  push:
    branches: [main]      # never the farm branch, or it triggers itself forever
  schedule:
    - cron: "17 4 * * *"
  workflow_dispatch:

permissions:
  contents: write         # the only permission it needs

jobs:
  grow:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0  # git-farm reads the whole history

      - uses: bright98/git-farm@<commit-sha>
        id: farm
        with:
          out: farm.svg
          theme: both     # or full, for the painted one
          since: 5y

      - name: publish
        env:
          SVG: ${{ steps.farm.outputs.svg }}
          DARK: ${{ steps.farm.outputs.dark-svg }}
        run: |
          set -euo pipefail
          files=("$SVG")
          if [ -n "$DARK" ]; then
            files+=("$DARK")
          fi
          git config user.name  "git-farm"
          git config user.email "git-farm@users.noreply.github.com"
          git checkout --orphan farm
          git rm -rf --cached . > /dev/null
          git add -f "${files[@]}"
          git commit -m "farm $(date -u +%F)"
          git push -f origin farm
```

Then point your `README.md` at the branch it publishes — with `both`:

```markdown
<picture>
  <source media="(prefers-color-scheme: dark)"
          srcset="https://raw.githubusercontent.com/USER/REPO/farm/farm-dark.svg">
  <img alt="the farm" src="https://raw.githubusercontent.com/USER/REPO/farm/farm.svg">
</picture>
```

or, with `full`, the one file is the whole picture:

```markdown
<img alt="the farm" src="https://raw.githubusercontent.com/USER/REPO/farm/farm.svg">
```

Run it once by hand from the Actions tab, and the picture is there.

**Two looks.** `both` writes a light file and a dark one, painting no
background, so the farm sits *in* your README and borrows the page. `full`
paints its own world — sky, clouds, tilled soil, wooden fences — and writes a
single file that looks the same on any page. Both, side by side and in day and
night, are on [the site](https://bright98.github.io/git-farm/).

## What it measures, and how much to trust it

Everything comes from one command — `git log --reverse --numstat` — plus the
list of files at HEAD. No GitHub API: file churn over 5,000 commits is one local
command or several thousand rate-limited requests, and the local one also works
on GitLab, on a folder with no remote, and inside a CI checkout that has the
history on disk already.

**Ranks, never fixed numbers.** "More than 40 commits is a hotspot" makes a
small repo look immaculate and a large one look doomed. Everything is a
percentile inside the repo it came from — with one guard: when a distribution is
flat, nothing is a peak. A repo where every file has been committed once by one
person grows no weeds.

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
is no code there to test. When no rule applies, the field is drawn with crop
marks at its corners instead of a fence — the extent is still clear, and nothing
has been claimed. `tests: [...]` in the config adds globs for a layout none of
the built-in rules knows about.

**Time is measured back from the newest commit, not from today.** `--since 5y`
means five years before HEAD, and "untouched for a year" means a year before
HEAD. Two runs a week apart over the same commit produce the same farm, which is
what lets the Action push nothing when nothing changed.

**A field with more files than squares shows a sample that keeps the
proportions**, with one rule on top: a kind that exists at all gets at least one
square. The picture will over-state two weeds among three hundred files, and
that is the intended direction to be wrong in.

**`--night=auto` reads the author's own clock,** not UTC: "committed after
midnight" is a claim about a person, and somebody committing at 02:14 in Tehran
did it after midnight even though the same instant is the previous evening in
London.

**`--weather` is off, and should stay off on anything public.** With it, a quiet
stretch draws the farm at night — which puts a "this project is dead" badge on
somebody's README without them ever asking for one.

## Flags

| Flag | Default | |
|---|---|---|
| `--theme` | `quiet` | `quiet`, `quiet-light`, `full`, or `both`; `--out` defaults to `quiet-light` |
| `--out` | | write an SVG here instead of drawing in the terminal |
| `--png` | | write a PNG here, for somewhere that will not take an SVG |
| `--watch` | | open the farm in a window you can walk around in |
| `--gif` | | write the history here as an animated GIF |
| `--frames` | `120` | at most this many frames in the GIF |
| `--weather` | `false` | let the sky report how busy the repository was |
| `--night` | `false` | `true`, `false`, or `auto` |
| `--names` | `true` | directory names on the fields, and who committed last |
| `--scale` | `6` | SVG units per pixel of the farm |
| `--color` | `auto` | `auto`, `full`, `256`, `16`, `none` |
| `--no-color` | | shorthand for `--color none`; `NO_COLOR` does the same |
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

`--png` is for a preview card, a slide, or a chat that pastes an image. It is
pixels and nothing else, so it has no directory names and only the painted
theme's fence survives — which is why it draws `full` unless told otherwise.
The SVG is the better file everywhere that takes one.

`--watch` moves a cursor between fields by direction; `enter` opens one and
walks its files worst first, `esc` goes back, `t` swaps themes, `n` is night,
`q` quits.

## The Action

| Input | Default | |
|---|---|---|
| `out` | `farm.svg` | where to write it; the dark file goes beside it |
| `theme` | `both` | `quiet`, `quiet-light`, `full`, or `both` |
| `since` | `5y` | ignore history older than this |
| `depth` | | how many path segments a field name keeps |
| `names` | `true` | `false` draws the farm with nobody named |
| `night` | `false` | `true`, `false`, or `auto` |
| `version` | | the release to run; empty means the one the action ships with |

**The branch is the part to get right.** Committing `farm.svg` to `main` on
every push grows the repository forever and fills its history with "update
farm". The `farm` branch the publish step makes is an orphan, force-pushed:
always exactly one commit holding two files, and `main` never hears about it.

**Pin to a commit SHA, not a tag.** A tag can be moved and a SHA cannot, and
this is an action that runs on your repository with `contents: write`. The
action downloads a released binary and checks it against the release's
`checksums.txt` before running it — a download that does not match is a failed
job, not a farm.

**Scheduled workflows are disabled after 60 days of no activity.** That is
GitHub, not a bug here: a repository nobody touches stops growing its farm.

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

## Speed, and what leaves the machine

The parsed repository is cached inside `.git/git-farm/`, keyed on the HEAD SHA
and on every option that changes what gets read. Homebrew's own repository —
22,403 commits and 5,582 files inside the default five-year window — takes about
11 seconds cold and 0.06 seconds warm. The cache deliberately does not cover the
working tree, which is read for line counts and inline tests; `--no-cache` is
the way past that.

Nothing leaves the machine. git-farm makes no network calls at all — it runs
`git log`, reads files in the checkout, and writes a picture. That is checkable
with one grep, and it is the reason it is worth running on somebody else's
repository. The Action reaches the network exactly once, to GitHub's own release
host. The picture shows directory paths and the name of whoever committed last;
`--names=false` removes both.

## Building it

```sh
go build ./cmd/git-farm && ./git-farm
go test ./...
```

The packages, how the drawing is tested, and how a release is cut are in
[CONTRIBUTING.md](CONTRIBUTING.md).

## License

MIT.
