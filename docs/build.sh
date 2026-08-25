#!/bin/sh
# Build the site: the page, and every picture on it drawn by this checkout.
#
# The workflow calls this and so can you, which is the point. A page whose
# pictures only exist in CI cannot be looked at before it is published —
# opening docs/index.html straight from disk shows the words with every image
# broken, because the pictures are generated, never committed.
#
#   ./docs/build.sh                 build into _site
#   ./docs/build.sh _site 8000      build, then serve it
set -eu

cd "$(dirname "$0")/.."
out=${1:-_site}
port=${2:-}

# The XXXXXX matters: BSD mktemp takes a bare prefix and GNU mktemp refuses
# one, so a template that works on a laptop can fail on the runner.
bin=$(mktemp -t git-farm.XXXXXX)
trap 'rm -f "$bin"' EXIT
go build -o "$bin" ./cmd/git-farm

mkdir -p "$out"
cp docs/index.html "$out/"

# --night=auto with the value attached: --night reports itself boolean so that
# a bare --night still means yes, and a boolean flag never consumes the next
# argument. Spelled with a space, "auto" would become the path.
"$bin" --out "$out/farm.svg" --theme full --night=auto .
"$bin" --out "$out/farm-quiet.svg" --theme both .
"$bin" --gif "$out/history.gif" .

echo "built $out"
[ -n "$port" ] || exit 0
echo "serving on http://localhost:$port"
cd "$out" && exec python3 -m http.server "$port"
