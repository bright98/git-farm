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

# Day and night for both themes, because the page has a switch and because the
# night farm is otherwise something the page describes and never shows.
#
# Explicitly day and explicitly night rather than auto: a switch labelled "day"
# that drew the night farm — which is what auto would do after midnight — is a
# switch that lies. Auto is what the README uses and what the page explains.
#
# The value is attached to --night, always: it reports itself boolean so that a
# bare --night still means yes, and a boolean flag never consumes the next
# argument. Spelled with a space, "auto" would become the path.
"$bin" --out "$out/farm.svg"       --theme full --night=false .
"$bin" --out "$out/farm-night.svg" --theme full --night=true  .

# All four spelled out, rather than two runs of --theme both. "both" pairs a
# light page with day and a dark page with night on purpose — a dark README
# wants a moon — so it can never produce the dark palette by day, which is
# exactly one of the four squares the switches can ask for.
#
# quiet is the palette for a dark page; quiet-light is the one for a light page.
"$bin" --out "$out/quiet-light-day.svg"   --theme quiet-light --night=false .
"$bin" --out "$out/quiet-light-night.svg" --theme quiet-light --night=true  .
"$bin" --out "$out/quiet-dark-day.svg"    --theme quiet       --night=false .
"$bin" --out "$out/quiet-dark-night.svg"  --theme quiet       --night=true  .

# The preview card, which is the one picture here that is not for this page.
# A card renderer takes PNG, JPEG or GIF and silently drops an SVG, so the
# page's own farm.svg cannot be its og:image.
#
# 190x50 rather than the default 120x50: a card is cropped to about 1.91:1,
# and cropping a farm top and bottom takes the sky off it and the front row
# with it. Wider costs nothing — the treemap simply draws more of the
# directories it was already gathering into other/.
"$bin" --png "$out/og.png" --theme full --night=false --width 190 --height 50 .

"$bin" --gif "$out/history.gif" .

echo "built $out"
[ -n "$port" ] || exit 0
echo "serving on http://localhost:$port"
cd "$out" && exec python3 -m http.server "$port"
