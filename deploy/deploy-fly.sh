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

echo "Deploying lioctad @ ${GIT_REV} to Fly..."
exec fly deploy --build-arg "GIT_REV=${GIT_REV}" "$@"
