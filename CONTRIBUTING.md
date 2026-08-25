# Working on git-farm

```sh
go build ./cmd/git-farm && ./git-farm
go test ./...
./docs/build.sh _site 8000    # the site, built and served locally
```

The site's pictures are generated and never committed, so opening
`docs/index.html` straight from disk shows the words with every image broken.
`docs/build.sh` is what the pages workflow runs, so what you look at locally and
what gets published cannot drift apart.

## The packages

| | |
|---|---|
| `cmd/git-farm/` | the command; named so that `git farm` works from the PATH |
| `internal/gitlog/` | runs `git log --numstat` and stream-parses it, including renames |
| `internal/repo/` | files rolled into directories, and the four kinds decided |
| `internal/cache/` | the parsed repo, keyed on HEAD |
| `internal/tui/` | the farm you can walk around in: `--watch` |
| `farm/` | the canvas, the themes, the treemap, the drawing and the SVG |
| `docs/` | the site, and the script that builds it |
| `action.yml` | the composite Action: downloads a released binary, checks it, runs it |
| `.goreleaser.yaml` | the release build — linux and darwin, amd64 and arm64 |

## Testing the picture

The picture is tested as letters, not as colours: `farm/dump_test.go` prints the
canvas one character per role, which is what makes a layout bug visible in a
test log rather than invisible in a byte diff.

Run it over repositories nobody wrote it against. Every drawing bug found that
way has been silent — a picture came out, and it was wrong — and several of the
constants in `farm/` exist because of one:

- **The minimum field size is the theme's own arithmetic, not one number.** It
  used to be a single constant measured against the quiet terminal's four-pixel
  plants. The file themes grow plants half again as tall, so a directory with
  six files in it was drawn as a fenced rectangle with a name on it and bare
  soil inside.
- **A short field takes the small farmer rather than none.** Same cause: the
  file themes' farmer is taller, so a field with room for a person on screen
  could have none in the SVG — and the farmer is the only thing in the file that
  moves, so the animation vanished with them.
- **The farmer belongs to the newest commit that *touched* something.** A
  release commit can be an empty commit; bubbletea's HEAD is one. Read literally
  it left the farm with nobody in it. The clock still comes from the newest
  commit.
- **`--out` with no `--theme` writes the light palette.** The quiet theme is
  built for a dark terminal, which is the wrong default for a file: a README
  page is light until its reader says otherwise.

## Releases

Tag `vN.N.N` and push it; `.github/workflows/release.yml` runs goreleaser.

`action.yml` pins `DEFAULT_VERSION` to the release it downloads, and the
workflow refuses to release a tag that does not match it — otherwise a new tag
would ship an action that quietly keeps running the previous binary.
