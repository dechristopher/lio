package www

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/dechristopher/lio/assets"
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/demo"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/og"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/handlers"
	"github.com/dechristopher/lio/www/handlers/api"
	"github.com/dechristopher/lio/www/middleware"
	"github.com/dechristopher/lio/www/ws"
)

var staticFs fs.FS

// Serve all public endpoints. Page rendering is handled by the typed templ
// components in the view package (see view.Render), so no template engine is
// configured on the fiber app.
func Serve(static embed.FS) {
	util.Info(str.CMain, str.MInit, config.Version)

	// make filesystem location decision based on environment
	staticFs = util.PickFS(env.IsLocal(), static, "./static")

	// content-hash the static assets so their URLs bust the cache exactly when
	// their bytes change; stable across instances (see the assets package).
	if err := assets.Build(staticFs); err != nil {
		util.Error(str.CMain, "asset manifest build failed: %s", err.Error())
	}

	// hand the OpenGraph card renderer the same static FS so link previews
	// composite the exact board/piece art the game client serves
	if err := og.LoadAssets(staticFs); err != nil {
		util.Error(str.CMain, "og card asset load failed: %s", err.Error())
	}

	r := fiber.New(fiber.Config{
		// bare product name — no version. The version is still available where it's
		// useful (the internal health listener's status JSON and the site footer);
		// keeping it out of the Server header on every response denies casual
		// version fingerprinting.
		ServerHeader:  "lioctad.org",
		CaseSensitive: true,
		ErrorHandler:  nil,
		// Connection hygiene against slow-loris style hoarding. ReadTimeout bounds
		// how long a client may take to send its request; IdleTimeout bounds a
		// kept-alive connection between requests. No WriteTimeout — it would cut
		// long responses, and the realtime path is WebSocket anyway (hijacked, so
		// these HTTP timeouts don't apply to it; the socket sets its own deadlines).
		ReadTimeout: 10 * time.Second,
		IdleTimeout: 65 * time.Second,
		// The largest legitimate body is a small create-game form; 64KiB is a
		// generous ceiling that caps oversized/abusive POSTs (default is 4MiB).
		BodyLimit: 64 * 1024,
	})

	// wire up all route handlers
	wireHandlers(r, staticFs)

	// Graceful shutdown with SIGINT
	// SIGTERM and others will hard kill
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		_ = <-c
		util.Info(str.CMain, str.MShutdown)
		// shutdown drain (arch/STATE_PERSISTENCE_SCALING.md): gate inbound
		// mutations, freeze clocks, flush final room snapshots — then tell
		// every client this is a restart (1012 Service Restart; the browser
		// surfaces the code in onclose and lio.js reconnects promptly instead
		// of treating it as a network failure). Fiber's Shutdown does not
		// touch hijacked websocket connections, so the CloseAll sweep is what
		// actually releases them.
		room.Drain()
		channel.CloseAll(1012, "server restarting")
		_ = r.Shutdown()
	}()

	util.Info(str.CMain, str.MStarted, util.TimeSinceBoot(),
		env.GetEnv(), config.GetPort(), config.GetHealthAddr())

	// loopback-only status listener for container health checks (see health.go)
	go serveHealth()

	// listen for connections on primary listening port
	if err := r.Listen(config.GetListenPort(), fiber.ListenConfig{
		DisableStartupMessage: true,
	}); err != nil {
		log.Println(err)
	}

	// Exit cleanly
	util.Info(str.CMain, str.MExit)
	os.Exit(0)
}

// wireHandlers builds all the websocket and http routes
// into the fiber app context
func wireHandlers(r *fiber.App, staticFs fs.FS) {
	// recover from panics
	r.Use(recover.New())

	// defensive response headers (CSP, framing, nosniff, HSTS, …) on every
	// response, error pages and redirects included — wired before anything that
	// can short-circuit so nothing escapes without them
	r.Use(middleware.SecurityHeaders())

	// stateless CSRF defense: reject cross-site POST/PUT/PATCH/DELETE. No-ops on
	// safe methods (the WS upgrade + all page/asset GETs pass through).
	r.Use(middleware.MutationGuard())

	// Configure CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins(),
		AllowHeaders: []string{"Origin", "Content-Type", "Accept"},
	}))

	// evaluate / set user context
	// TODO rebuild this every time someone logs in
	r.Use(user.ContextMiddleware)

	// websocket upgrade middleware
	r.Use("/socket", ws.UpgradeHandler)

	// websocket connection listener
	r.Get("/socket/:chan", ws.ConnHandler())
	// websocket
	r.Get("/socket/:type/:chan", ws.ConnHandler())

	// sub-router with compression and other middleware enabled
	sub := r.Group("/")

	// wire up all middleware components
	middleware.Wire(sub, staticFs)

	// group for /api routes
	apiGroup := sub.Group("/api")

	// wire all the api handlers
	api.Wire(apiGroup)

	// home handler
	// TODO not needed once we default SPAHandler
	r.Get("/", handlers.IndexHandler)

	// live home-activity fragment polled by htmx (stats / challenges / live games)
	r.Get("/home/activity", handlers.HomeActivityHandler)

	// random demo games for the home-page "What is Octad?" self-playing board.
	// Warm the game pool off the request path so the first visitor doesn't pay
	// the one-time build (Batch also builds lazily as a fallback).
	r.Get("/home/demo", handlers.HomeDemoHandler)
	go demo.WarmPool()

	// other pages
	r.Get("/about", handlers.AboutHandler)
	r.Get("/about/board", handlers.AboutBoardHandler)
	r.Get("/about/rules", handlers.AboutRulesHandler)
	r.Get("/about/notation", handlers.AboutNotationHandler)
	r.Get("/about/misc", handlers.AboutMiscHandler)

	// paginated news feed page
	r.Get("/news", handlers.NewsHandler)

	// game database page handler
	r.Get("/db", handlers.DBHandler)

	// OpenGraph preview cards (the og:image targets scrapers fetch when a
	// lioctad link is shared): the site-wide default card and the per-room
	// live-position card
	r.Get("/og/default.png", handlers.OGDefaultHandler)
	r.Get("/og/room/:id", handlers.OGRoomHandler)

	// new room creation routes. All POST (never GET): creating a room is a
	// state change, and a GET would be CSRF-able via a top-level cross-site
	// navigation (SameSite=Lax attaches cookies to those). The group is
	// rate-limited per client IP so a script can't spin up unbounded rooms.
	newRoom := r.Group("/new", middleware.RoomCreateLimiter())
	newRoom.Post("/game", handlers.NewCustomRoom)
	newRoom.Post("/human/quick", handlers.NewQuickRoomVsHuman)
	newRoom.Post("/computer", handlers.NewRoomVsComputer)

	// direct archived-game permalink by UUID (301s to the canonical
	// /<room_id>/<n> when the game has a room). Registered before the room
	// wildcards so "game" is never captured as a room id.
	r.Get("/game/:uuid", handlers.ArchiveGameByUUIDHandler)

	// archived-game data for in-room match browsing (immutable JSON; see
	// ArchiveGameJSONHandler)
	r.Get("/api/room/:id/game/:num", handlers.ArchiveGameJSONHandler)

	// room handlers. /:id serves the live room while its actor exists and
	// falls back to the archived match view once it's gone; /:id/:num is the
	// permanent per-game permalink (1-based match ordinal)
	r.Get("/:id", handlers.RoomHandler)
	r.Get("/:id/:num", handlers.ArchiveGameHandler)
	r.Post("/:id/join", handlers.RoomJoinHandler)
	r.Post("/:id/cancel", handlers.RoomCancelHandler)

	// return static index.html for all other paths and let
	// React handle 404s so that we get nice error pages
	//r.Get("/*", handlers.SPAHandlerInit(staticFs))

	// Custom 404 page
	// TODO not needed once we default SPAHandler
	middleware.NotFound(r)
}

// corsOrigins splits the comma-separated CORS origin list (config.CorsOrigins)
// into the []string that fiber v3's cors middleware expects.
func corsOrigins() []string {
	parts := strings.Split(config.CorsOrigins(), ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if s := strings.TrimSpace(p); s != "" {
			origins = append(origins, s)
		}
	}
	return origins
}
