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

| On the farm | In the repo |
|---|---|
| a fenced field | the directory has tests |
| a broken fence | no test files were found |
| a green upright plant | a file changed recently |
| a tall plant | a big file |
| **weeds** | **churn: many authors, many commits, same file** |
| a grey thin plant | untouched for years |
| a flat mark at ground level | a file that was deleted |
| the farmer | the author of the newest commit, standing in that field |

## Status

**Phase 0 of [the plan](git-farm-plan.md): it reads repositories, it does not
draw them yet.** What works today is everything underneath the picture — the
history, the classification, and `--json`. The drawing layer exists separately,
in [`git-farm-demo/`](git-farm-demo/), where it draws a made-up repo.

```sh
go build ./cmd/git-farm
./git-farm                    # what this repo looks like, in words
./git-farm --json             # the parsed repo, which is what the picture is drawn from
./git-farm ~/src/some-repo    # any other repo
```

```
~/src/ssh-games
8 commits, 1 author, 72 files, less than a day of history
newest commit by Haleh in internal/games/

field             kind      tests    files  weeds    big    dry
cmd/arcade/       untested  none         1      0      0      0
internal/store/   healthy   found       11      4      2      0
internal/ui/      healthy   found       13      5      2      0
migrations/       healthy   unknown      6      0      0      0
```

## What it measures, and how much to trust it

Everything comes from one command — `git log --reverse --numstat` — plus the
list of files at HEAD. No GitHub API: file churn over 5,000 commits is one
local command or several thousand rate-limited requests, and the local one also
works on GitLab, on a folder with no remote, and inside a CI checkout that has
the history on disk already.

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
answer is `unknown` and the fence is drawn as unknown rather than broken.

`tests: [...]` in the config adds globs for a layout none of the built-in rules
knows about.

**Time is measured back from the newest commit, not from today.** `--since 5y`
means five years before HEAD, and "untouched for a year" means a year before
HEAD. Two runs a week apart over the same commit therefore produce the same
farm, which is what lets the eventual Action push nothing when nothing changed.

## Refusals

Three cases produce a farm that is quietly wrong, so they produce an error
instead:

- **not a git repository**
- **a repository with no commits**
- **a shallow clone** — somebody forgot `fetch-depth: 0` and got one commit of
  history, which makes every file look new and every directory look like a
  hotspot. The message says how to fix it.

## Flags

| Flag | Default | |
|---|---|---|
| `--json` | | print the parsed repo and stop; the debugging escape hatch |
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
counts and inline tests. An uncommitted edit will not invalidate it; `--no-cache`
is the way past that.

## Safety

Nothing leaves the machine. git-farm makes no network calls at all — it runs
`git log`, reads files in the checkout, and writes a picture. That is checkable
with one grep.

## Layout

| | |
|---|---|
| `cmd/git-farm/` | the command; named so that `git farm` works from the PATH |
| `internal/gitlog/` | runs `git log --numstat` and stream-parses it, including renames |
| `internal/repo/` | files rolled into directories, and the four kinds decided |
| `internal/cache/` | the parsed repo, keyed on HEAD |
| `git-farm-demo/` | the drawing layer, on a made-up repo — a separate module |

## License

MIT.
