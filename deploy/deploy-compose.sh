#!/bin/bash

# Deploy/update the lioctad docker-compose stack (deploy/docker-compose.yaml).
#
# The compose replacement for deploy-fly.sh: resolves the short commit hash of
# the checked-out tree and passes it through as GIT_REV so the build injects it
# into config.Revision (footer shows v0.9.x+<hash>), then rebuilds the image
# and rolls the service. Safe to re-run — compose only recreates the container
# when the image or config actually changed.
#
# If the new build never passes its healthcheck, the deploy self-heals instead
# of leaving a poisoned environment: it dumps the failing container's logs,
# retags the previously running image back to lioctad:latest, and rolls the
# service back (verifying the old build comes back healthy). On a first deploy
# with nothing to roll back to, the broken service is stopped instead.
#
# Every healthy deploy also tags its image lioctad:<git_rev>, so a manual
# rollback to any earlier build is:
#   docker tag lioctad:<rev> lioctad:latest && docker compose -f deploy/docker-compose.yaml up -d
#
# Run from anywhere; extra args are passed to `docker compose up`
# (e.g. deploy/deploy-compose.sh --force-recreate).

set -euo pipefail

cd "$(dirname "$0")"

COMPOSE=(docker compose -f docker-compose.yaml)

# refuse to deploy with missing/empty secret files — the app would either fail
# to boot (object store) or silently run with a blank crypto key
for s in crypto_key lio_obj_endpoint lio_obj_bucket_pgn lio_obj_access lio_obj_secret; do
	if [ ! -s "secrets/$s" ]; then
		echo "error: secret file deploy/secrets/$s is missing or empty" >&2
		exit 1
	fi
done

# resolve the short commit hash of the tree being deployed (the build context
# is src/ with no .git, so the build cannot compute this itself)
GIT_REV="$(git rev-parse --short HEAD)"
export GIT_REV

# wait_healthy <label> — poll the lioctad service healthcheck for up to 60s
wait_healthy() {
	echo -n "Waiting for lioctad ($1) to become healthy"
	for _ in $(seq 1 30); do
		status="$("${COMPOSE[@]}" ps --format '{{.Health}}' lioctad 2>/dev/null || true)"
		if [ "$status" = "healthy" ]; then
			echo
			return 0
		fi
		echo -n "."
		sleep 2
	done
	echo
	return 1
}

# emit_failure_logs <label> — surface why the container is unhealthy
emit_failure_logs() {
	{
		echo "---- lioctad ($1) failed its healthcheck; container status:"
		"${COMPOSE[@]}" ps lioctad
		echo "---- last 100 log lines:"
		"${COMPOSE[@]}" logs --tail 100 --no-color lioctad
		echo "----"
	} >&2
}

# capture the image the currently running container was started from, *before*
# the build retags lioctad:latest out from under it — this exact image ID is
# the rollback target (may be empty on a first deploy)
PREV_IMAGE="$("${COMPOSE[@]}" ps -q lioctad 2>/dev/null \
	| head -n1 \
	| xargs -r docker inspect -f '{{.Image}}' 2>/dev/null || true)"

echo "Deploying lioctad @ ${GIT_REV} via docker compose..."
"${COMPOSE[@]}" build --pull
"${COMPOSE[@]}" up -d "$@"

if wait_healthy "new build @ ${GIT_REV}"; then
	# keep a per-revision tag around for manual rollbacks/audit
	docker tag lioctad:latest "lioctad:${GIT_REV}"
	echo "lioctad is healthy @ ${GIT_REV}"
	"${COMPOSE[@]}" ps
	exit 0
fi

emit_failure_logs "new build @ ${GIT_REV}"

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

if wait_healthy "rollback"; then
	echo "rollback succeeded — previous build is healthy again; deploy of ${GIT_REV} FAILED" >&2
	"${COMPOSE[@]}" ps
	exit 1
fi

emit_failure_logs "rollback"
echo "error: rollback image did not become healthy either; stopping the service" >&2
"${COMPOSE[@]}" stop lioctad
exit 2
