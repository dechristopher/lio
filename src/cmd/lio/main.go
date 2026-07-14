package main

import (
	"embed"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dechristopher/lio/crypt"
	"github.com/joho/godotenv"

	"github.com/dechristopher/lio/backfill"
	"github.com/dechristopher/lio/cache"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/systems"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www"
)

// runBackfill is the --backfill flag: replay the PGN archive into Postgres and
// exit, rather than serving. Parsed in init, consumed in main (it needs the
// object store + db up first).
var runBackfill *bool

var (
	//go:embed static/*
	static embed.FS
)

// init parses flags, sets constants, and prepares us for battle
func init() {
	// set boot time immediately
	config.BootTime = time.Now()

	// parse command line flags
	isHealthCheck := flag.Bool(str.FHealth, false, str.FHealthUsage)
	runBackfill = flag.Bool(str.FBackfill, false, str.FBackfillUsage)
	config.DebugFlagPtr = flag.String(str.FDebugFlags, "", str.FDebugFlagsUsage)
	flag.Parse()

	// parse out debug flags from command line options
	for _, debugFlag := range strings.Split(*config.DebugFlagPtr, ",") {
		config.DebugFlags[debugFlag] = true
	}

	// run health check if told (this exits the process; the server never starts)
	if *isHealthCheck {
		executeHealthCheck()
		return
	}

	if !env.IsProd() {
		// print development mode warning
		util.Debug(str.CMain, str.MDevMode)
	}

	// test that crypto system is operational
	_, _ = crypt.Encrypt([]byte("lio"))
}

// executeHealthCheck probes the running server's internal health listener and
// exits the process 0 (healthy) or non-zero (unhealthy). It is the container
// HEALTHCHECK: the scratch runtime image has no shell or wget, so the binary
// must check itself. It hits /lio on the loopback-only health listener (see
// www/health.go — JSON status, no side effects, no object-store dependency,
// never exposed outside the container) and never starts the server or its
// subsystems.
func executeHealthCheck() {
	client := http.Client{Timeout: 3 * time.Second}

	resp, err := client.Get("http://" + config.GetHealthAddr() + "/lio")
	if err != nil {
		util.Error(str.CMain, "health check failed: %s", err.Error())
		os.Exit(1)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		util.Error(str.CMain, "health check failed: status %d", resp.StatusCode)
		os.Exit(1)
	}

	os.Exit(0)
}

// main does the things
func main() {
	// load .env if any
	_ = godotenv.Load()

	// initialize subsystems synchronously: the bus must be online before any
	// room exists (clock flips publish to it), and everything below depends on
	// config/secrets being readable. The chain is fast — the only network touch
	// is the object store's credential check.
	systems.Run()

	// one-off archive backfill: replay the object-store PGN archive into
	// Postgres, then exit without serving. Runs after systems.Run (object store
	// + db up, migrations applied) and is meant to be a throwaway container:
	//   docker compose run --rm lioctad --backfill
	if *runBackfill {
		if err := backfill.Run(); err != nil {
			log.Fatalln(str.CMain, err.Error())
		}
		os.Exit(0)
	}

	// restart persistence (arch/STATE_PERSISTENCE_SCALING.md): bring the cache
	// online, restore persisted rooms, then start the write-behind persister.
	// Rehydration MUST complete before the listener below accepts connections,
	// or reconnecting clients race the restore and get bounced as "room gone".
	cache.Up()
	if cache.Ready() {
		room.RehydrateAll(cache.RoomSnapshots{})
		room.UpPersister(cache.RoomSnapshots{})
	}

	// optional background position evaluator (fills the deduped positions eval
	// cache off the game path; no-op unless Postgres + the evaluator are enabled)
	db.UpEvaluator()

	// serve primary http endpoints
	www.Serve(static)
}
