#!/usr/bin/env bash
#
# Build the gitignored frontend assets into src/cmd/lio/static/ for local dev,
# mirroring what src/Dockerfile does at deploy time. Run once after a fresh clone
# (and after editing view/app.css or bumping the octadground pin) so
# `go run ./cmd/lio` serves a styled page with a working board.
#
#   bin/build-assets.sh            # css always; octadground only if missing
#   bin/build-assets.sh --force    # also rebuild the octadground bundle
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root/src"

tw="$root/bin/tailwindcss"
static="cmd/lio/static"

# 1) Tailwind CSS — rebuilt every run (the source changes often).
if [[ ! -x "$tw" ]]; then
  echo "error: $tw not found; download the standalone CLI (see CLAUDE.md)" >&2
  exit 1
fi
echo "==> building app.css"
"$tw" -i view/app.css -o "$static/app.css" --minify

# 2) octadground bundle — rebuilt only when missing or --force, from the same
#    pinned ref the Dockerfile uses so local matches deploy.
if [[ "${1:-}" == "--force" || ! -f "$static/octadground.js" ]]; then
  ref="$(grep -oE 'OCTADGROUND_REF=[^ ]+' Dockerfile | head -1 | cut -d= -f2)"
  echo "==> building octadground @ ${ref}"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' EXIT
  git clone --depth 1 --branch "$ref" https://github.com/dechristopher/octadground.git "$tmp"
  ( cd "$tmp" && yarn install --frozen-lockfile && yarn dist )
  cp "$tmp/dist/octadground.min.js" "$static/octadground.js"
  cp "$tmp/assets/octadground.base.css" "$static/octadground.base.css"
else
  echo "==> octadground.js present; skipping (use --force to rebuild)"
fi

echo "==> done"
