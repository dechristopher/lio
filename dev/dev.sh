#!/usr/bin/env bash
#
# Local development stack helper for lioctad. Runs the backing services
# (Postgres + Redis, MinIO optional) in Docker with persistent named volumes;
# run the lio server itself natively (GoLand "lio-local" run config or
# `go run ./cmd/lio`) against them. See dev/README.md.
#
#   dev/dev.sh up | down | reset | reset-db | minio | psql | redis-cli | logs | status | env
set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
compose=(docker compose -f "$here/docker-compose.yaml")
project="lio-dev"

usage() {
  cat <<'EOF'
usage: dev/dev.sh <command>

  up          start postgres + redis, wait until healthy
  down        stop services (volumes/data preserved)
  reset       wipe ALL volumes and restart from scratch (fresh slate)
  reset-db    wipe only the postgres volume (fresh games/analytics schema)
  minio       start including the optional MinIO object store (+ pgn bucket)
  psql        open a psql shell on the dev database
  redis-cli   open a redis-cli shell
  logs        follow service logs
  status      show service status
  env         print a ready-to-paste src/cmd/lio/.env block (generates a key)

The lio server runs natively (not in compose): after `up`, boot it from the IDE
or with `cd src && go run ./cmd/lio`. Migrations apply automatically at boot.
EOF
}

cmd="${1:-}"
case "$cmd" in
  up)
    "${compose[@]}" up -d --wait
    echo "postgres  → localhost:5432  (user/pass lio, db lio)"
    echo "redis     → localhost:6379"
    ;;
  down)
    "${compose[@]}" down
    ;;
  reset)
    "${compose[@]}" down -v
    "${compose[@]}" up -d --wait
    echo "fresh slate: all volumes wiped; migrations re-apply on next app boot"
    ;;
  reset-db)
    "${compose[@]}" rm -sf postgres >/dev/null
    docker volume rm "${project}_lio-dev-pg" >/dev/null 2>&1 || true
    "${compose[@]}" up -d --wait postgres
    echo "postgres volume wiped; migrations re-apply on next app boot"
    ;;
  minio)
    "${compose[@]}" --profile minio up -d --wait
    echo "minio     → localhost:9000 (console :9001, lioadmin/liosecret123, bucket lio-pgn)"
    ;;
  psql)
    "${compose[@]}" exec postgres psql -U lio -d lio
    ;;
  redis-cli)
    "${compose[@]}" exec redis redis-cli
    ;;
  logs)
    "${compose[@]}" logs -f
    ;;
  status)
    "${compose[@]}" ps
    ;;
  env)
    key="$(openssl rand -base64 24 2>/dev/null || true)"
    if [ -z "$key" ]; then
      key="$(LC_ALL=C tr -dc 'A-Za-z0-9' < /dev/urandom | head -c 32 || true)"
    fi
    cat <<EOF
# paste into src/cmd/lio/.env (DEV_CRYPTO_KEY must be a 16/24/32-byte string)
DEPLOY=local
PORT=4444
DEV_CRYPTO_KEY=${key}
DEV_LIO_REDIS_ADDR=localhost:6379
DEV_LIO_PG_DSN=postgres://lio:lio@localhost:5432/lio?sslmode=disable
EOF
    ;;
  *)
    usage
    [ -n "$cmd" ] && exit 1 || exit 0
    ;;
esac
