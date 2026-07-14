package db

import (
	"context"
	"database/sql"
	"embed"
	"log"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver for goose
	"github.com/pressly/goose/v3"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Pool is the Postgres connection pool, nil when no database is configured
// (local dev without lio_pg_dsn — the durable game archive is then disabled,
// mirroring store.C / cache.C). Postgres is the system of record for finished
// games only; it is never authoritative for live game state (that is Redis /
// the room actors).
var Pool *pgxpool.Pool

// opTimeout bounds routine archive/query round trips. The database lives on the
// compose/loopback network, so anything slower is an outage, not latency, and
// the archive path (a background goroutine off the move hot path) must never be
// held hostage by it.
const opTimeout = 5 * time.Second

// Up brings the Postgres pool online and applies migrations. Follows the
// store.Up / cache.Up degradation pattern: an unset DSN in local dev is fine
// (warn and run without the durable archive); in prod a missing or unreachable
// database — or a failed migration — refuses to boot, because silently serving
// without the archive would make finished games look durable while they aren't.
// Migrations run here (before the listener) so the schema is always current.
func Up() {
	dsn := config.ReadSecretFallback("lio_pg_dsn")

	if dsn == "" {
		if env.IsLocal() {
			util.Info(str.CDB, "no postgres configured; game archive disabled (local)")
			return
		}
		log.Fatalln(str.CDB, "no postgres dsn configured (lio_pg_dsn)")
	}

	ctx, cancel := context.WithTimeout(context.Background(), opTimeout)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fatalOrDegrade("postgres init failed", err)
		return
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		fatalOrDegrade("postgres unreachable", err)
		return
	}

	if err := runMigrations(dsn); err != nil {
		pool.Close()
		fatalOrDegrade("migrations failed", err)
		return
	}

	Pool = pool
	initGamesTotal()
	util.Debug(str.CDB, "postgres online")
}

// gamesTotal is an in-memory running count of archived games, shown on the home
// page. It is seeded once from count(*) at boot and incremented on each
// successful ArchiveGame, so the home-activity fragment (which self-refreshes
// every 5s) reads it without ever touching the database.
var gamesTotal atomic.Int64

// initGamesTotal seeds the games counter from the current table size.
func initGamesTotal() {
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).CountGames(ctx)
	if err != nil {
		util.Error(str.CDB, "games count init failed error=%s", err.Error())
		return
	}
	gamesTotal.Store(n)
}

// TotalGames returns the cached count of archived games (0 when Postgres is
// unconfigured).
func TotalGames() int64 {
	return gamesTotal.Load()
}

// fatalOrDegrade logs err as a fatal boot failure in prod, or a degraded-mode
// warning (archive disabled) in local dev — Pool stays nil either way locally.
func fatalOrDegrade(msg string, err error) {
	if env.IsLocal() {
		util.Error(str.CDB, msg+"; game archive disabled (local): %v", err)
		return
	}
	log.Fatalln(str.CDB, msg+":", err.Error())
}

// runMigrations applies the embedded goose migrations. goose speaks
// database/sql, so it opens a short-lived handle over the pgx stdlib driver
// (registered by the blank import above); app queries use the native pgxpool.
func runMigrations(dsn string) error {
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(migrationsFS)
	goose.SetLogger(goose.NopLogger()) // we log our own boot line
	if err := goose.SetDialect("postgres"); err != nil {
		return err
	}
	return goose.Up(sqlDB, "migrations")
}

// Ready reports whether a database connection is configured and was reachable
// (and migrated) at boot. Runtime outages after a healthy boot do not flip
// this: archive writes fail and are logged, but the server keeps running.
func Ready() bool {
	return Pool != nil
}

// Ctx returns a context bounded by the standard db op timeout, and its cancel.
func Ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), opTimeout)
}
