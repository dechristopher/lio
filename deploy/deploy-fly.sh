#!/bin/bash

# Deploy lioctad to Fly.io.
#
# Fly builds the Dockerfile with src/ as the build context, which does not
# include the repo's .git directory — so the build cannot compute its own commit
# hash. We resolve the short hash here and pass it in as the GIT_REV build-arg;
# the Dockerfile injects it into config.Revision via ldflags, and it surfaces in
# the site footer (v0.9.0+<hash>) so a deployed build is always identifiable.

set -euo pipefail

# resolve the short commit hash of the tree being deployed
GIT_REV="$(git rev-parse --short HEAD)"

# fly.toml lives in src/; deploy from there regardless of the caller's cwd
cd "$(dirname "$0")/../src"

# Build the minified Tailwind stylesheet into the tree Fly uploads. app.css is a
# generated artifact (gitignored); the Dockerfile only COPYs + embeds static/*,
# so the file must exist and be current at deploy time or prod ships unstyled.
if [ ! -x ../bin/tailwindcss ]; then
  echo "error: ../bin/tailwindcss not found; download the standalone CLI" >&2
  exit 1
fi
echo "Building minified app.css..."
../bin/tailwindcss -i view/app.css -o cmd/lio/static/app.css --minify

echo "Deploying lioctad @ ${GIT_REV} to Fly..."
exec fly deploy --build-arg "GIT_REV=${GIT_REV}" "$@"
