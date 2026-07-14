# Local development environment

A repeatable, ephemeral local stack for lioctad: Postgres + Redis (MinIO
optional) in Docker with **persistent named volumes**, plus a one-command fresh
slate. The lio server runs **natively** (GoLand or `go run`) so you keep fast
rebuilds and the debugger (compose only supplies the backing services). Service
versions mirror `deploy/docker-compose.yaml`, so local behavior matches prod.

Migrations run in-process at app boot, so "fresh slate" is just: wipe the
volumes, then boot the app (it re-creates the schema).

## First run

```bash
dev/dev.sh up                         # start postgres + redis (waits healthy)
cp src/cmd/lio/.env.example src/cmd/lio/.env
dev/dev.sh env                        # prints a DEV_CRYPTO_KEY + connection block
# paste the printed key into src/cmd/lio/.env
bin/build-assets.sh                   # build the gitignored frontend assets (once)
cd src && go run ./cmd/lio --debug room,dispatch,clock,engine,db
```

## In GoLand / IntelliJ

Shared run configs live in `.run/` (committed):

- **lio-local** — runs the server; its *Before launch* step starts **dev
  services (up)**, so one click brings up Postgres + Redis, applies migrations,
  and serves. Working dir is `src/cmd/lio`, so `.env` and the embedded assets
  resolve. (Run **build assets** once after a fresh clone.)
- **dev services (up)** / **dev reset (fresh slate)** — start / wipe-and-restart
  the stack.

## Commands

```
dev/dev.sh up          start postgres + redis, wait until healthy
dev/dev.sh down        stop services (data preserved)
dev/dev.sh reset       wipe ALL volumes and restart (fresh slate)
dev/dev.sh reset-db    wipe only the postgres volume (fresh games schema)
dev/dev.sh minio       start including the optional MinIO object store
dev/dev.sh psql        psql shell on the dev database
dev/dev.sh redis-cli   redis-cli shell
dev/dev.sh logs        follow logs
dev/dev.sh status      service status
dev/dev.sh env         print a ready-to-paste .env block (generates a key)
```

## Typical loops

- **Persist across restarts:** just stop/start the app — the volumes survive.
- **Fresh slate:** stop the app → `dev/dev.sh reset` → boot the app (re-migrates).
- **Inspect the archive:** `dev/dev.sh psql`, then e.g.
  `select id, outcome, method, reason from games order by id desc limit 10;`

## Notes

- Postgres/Redis publish to `127.0.0.1` only. Data lives in the `lio-dev-pg` /
  `lio-dev-redis` named volumes.
- Leaving `DEV_LIO_PG_DSN` / `DEV_LIO_REDIS_ADDR` unset disables the archive /
  restart-persistence locally (the server still runs) — the same graceful
  degradation as prod, minus the boot-fatal.
- MinIO is optional and behind the `minio` profile. `store.Up` uses TLS
  (`Secure:true`), so plain-http local MinIO won't connect without a code/TLS
  change; leaving `DEV_LIO_OBJ_ENDPOINT` empty (archival disabled) is the normal
  local setting and does not affect the Postgres archive.
