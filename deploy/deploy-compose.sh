#!/bin/bash

# Deploy/update the lioctad docker-compose stack (deploy/docker-compose.yaml).
#
# The compose replacement for deploy-fly.sh: resolves the short commit hash of
# the checked-out tree and passes it through as GIT_REV so the build injects it
# into config.Revision (footer shows v0.9.x+<hash>), then rebuilds the image
# and rolls the service. Safe to re-run — compose only recreates the container
# when the image or config actually changed.
#
# While the new build comes up its logs are streamed live, so a bad build can be
# triaged as it happens instead of only after the healthcheck times out.
#
# If the new build never passes its healthcheck, the deploy self-heals instead
# of leaving a poisoned environment: it retags the previously running image back
# to lioctad:latest and rolls the service back (verifying the old build comes
# back healthy). On a first deploy with nothing to roll back to, the broken
# service is stopped instead.
#
# Every healthy deploy tags its image "<version>-<commit>" (e.g.
# lioctad:v0.9.17-abc1234) so hotfix deploys at the same version don't clobber
# each other, and prunes older per-release images, keeping the most recent
# KEEP_RELEASES. Manual rollback to any retained build:
#   docker tag lioctad:<version>-<commit> lioctad:latest && docker compose -f deploy/docker-compose.yaml up -d
#
# Run from anywhere; extra args are passed to `docker compose up`
# (e.g. deploy/deploy-compose.sh --force-recreate).

set -euo pipefail

cd "$(dirname "$0")"

COMPOSE=(docker compose -f docker-compose.yaml)

# Seconds to wait for the healthcheck to go green before giving up and rolling
# back. Kept short: the app boots in milliseconds and the compose healthcheck is
# tuned to report quickly, so exceeding this means a broken build, not a slow one.
HEALTH_TIMEOUT_SECS=30

# Number of most-recent per-release images to retain when pruning after a deploy.
KEEP_RELEASES=3

# refuse to deploy with missing/empty secret files — the app would either fail
# to boot (object store) or silently run with a blank crypto key
for s in crypto_key lio_obj_endpoint lio_obj_bucket_pgn lio_obj_access lio_obj_secret; do
	if [ ! -s "secrets/$s" ]; then
		echo "error: secret file deploy/secrets/$s is missing or empty" >&2
		exit 1
	fi
done

# The runtime container runs unprivileged (uid 10001, see src/Dockerfile) and
# `docker compose` bind-mounts file secrets with their host permissions — so the
# files must be readable by that user or the app boots with a blank crypto key
# (crypto/aes: invalid key size 0). Keep the directory owner-only (host
# protection) but make the files other-readable; inside the container they are
# exposed only through the private per-service secret mount.
chmod 700 secrets 2>/dev/null || true
chmod o+r secrets/* 2>/dev/null \
	|| echo "warn: could not make secret files readable; ensure uid 10001 can read them" >&2

# resolve build identifiers (the build context is src/ with no .git, so the
# build cannot compute these itself):
#   GIT_REV  — short commit hash, injected into config.Revision (footer suffix)
#   RELEASE  — image tag, "<version>-<commit>" (e.g. v0.9.17-abc1234). VERSION is
#              the most recent tag *reachable from* HEAD (--abbrev=0), so a build
#              that is one or more commits past the tag — a hotfix, or just an
#              un-tagged deploy — still resolves to that version instead of
#              requiring HEAD to sit exactly on the tag. The -<commit> suffix
#              keeps successive same-version deploys from clobbering one image
#              tag. Falls back to just the commit when no tag is reachable yet.
GIT_REV="$(git rev-parse --short HEAD)"
VERSION="$(git describe --tags --abbrev=0 2>/dev/null || true)"
if [ -n "$VERSION" ]; then
	RELEASE="${VERSION}-${GIT_REV}"
else
	RELEASE="$GIT_REV"
fi
export GIT_REV

# wait_healthy <label> — poll the lioctad healthcheck for up to HEALTH_TIMEOUT_SECS
wait_healthy() {
	echo -n "Waiting for lioctad ($1) to become healthy"
	local waited=0
	while [ "$waited" -lt "$HEALTH_TIMEOUT_SECS" ]; do
		status="$("${COMPOSE[@]}" ps --format '{{.Health}}' lioctad 2>/dev/null || true)"
		if [ "$status" = "healthy" ]; then
			echo
			return 0
		fi
		echo -n "."
		sleep 2
		waited=$((waited + 2))
	done
	echo
	return 1
}

# Live-tail the container's logs during a health wait so a failing build can be
# triaged as it happens, rather than only after a rollback dumps them. The
# follower is a background process stopped once health resolves (and by the EXIT
# trap, so it never outlives the script).
LOG_FOLLOW_PID=""
start_log_follow() {
	"${COMPOSE[@]}" logs -f --no-color lioctad &
	LOG_FOLLOW_PID=$!
}
stop_log_follow() {
	if [ -n "$LOG_FOLLOW_PID" ]; then
		kill "$LOG_FOLLOW_PID" 2>/dev/null || true
		wait "$LOG_FOLLOW_PID" 2>/dev/null || true
		LOG_FOLLOW_PID=""
	fi
}
trap 'stop_log_follow' EXIT

# emit_failure_status <label> — the live logs already streamed above, so just
# surface the container's status/exit for the record
emit_failure_status() {
	{
		echo "---- lioctad ($1) failed its healthcheck; container status:"
		"${COMPOSE[@]}" ps -a lioctad
		echo "----"
	} >&2
}

# purge_old_images — keep :latest plus the KEEP_RELEASES most-recent per-release
# tags; remove older lioctad images so the host doesn't accumulate every build.
# `docker images` lists newest-first; never fatal to the deploy.
purge_old_images() {
	docker images --format '{{.Repository}}:{{.Tag}}' lioctad 2>/dev/null \
		| grep -Ev ':(latest|<none>)$' \
		| tail -n "+$((KEEP_RELEASES + 1))" \
		| xargs -r docker rmi 2>/dev/null || true
}

# capture the image the currently running container was started from, *before*
# the build retags lioctad:latest out from under it — this exact image ID is
# the rollback target (may be empty on a first deploy)
PREV_IMAGE="$("${COMPOSE[@]}" ps -q lioctad 2>/dev/null \
	| head -n1 \
	| xargs -r docker inspect -f '{{.Image}}' 2>/dev/null || true)"

echo "Deploying lioctad ${RELEASE} via docker compose..."
"${COMPOSE[@]}" build --pull
"${COMPOSE[@]}" up -d "$@"

start_log_follow
if wait_healthy "new build ${RELEASE}"; then
	stop_log_follow
	# tag this build with its release version for rollbacks/audit, then prune old ones
	docker tag lioctad:latest "lioctad:${RELEASE}"
	purge_old_images
	echo "lioctad is healthy @ ${RELEASE}"
	"${COMPOSE[@]}" ps
	exit 0
fi
stop_log_follow

emit_failure_status "new build ${RELEASE}"

if [ -z "$PREV_IMAGE" ]; then
	echo "error: deploy failed and no previous image exists to roll back to;" >&2
	echo "stopping the broken service" >&2
	"${COMPOSE[@]}" stop lioctad
	exit 1
fi

echo "Rolling back to previous image ${PREV_IMAGE#sha256:}..." >&2
docker tag "$PREV_IMAGE" lioctad:latest
# compose sees the changed image ID and recreates the container from the old
# build; --no-build so it can't helpfully rebuild the bad one
"${COMPOSE[@]}" up -d --no-build lioctad

start_log_follow
if wait_healthy "rollback"; then
	stop_log_follow
	echo "rollback succeeded — previous build is healthy again; deploy of ${RELEASE} FAILED" >&2
	"${COMPOSE[@]}" ps
	exit 1
fi
stop_log_follow

emit_failure_status "rollback"
echo "error: rollback image did not become healthy either; stopping the service" >&2
"${COMPOSE[@]}" stop lioctad
exit 2
