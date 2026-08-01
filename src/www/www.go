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
	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/demo"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/home"
	"github.com/dechristopher/lio/og"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
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
		ServerHeader:  config.SiteName(),
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

	// Hand the home hub its digest source (arch/HOME_ACTIVITY_STREAMING.md).
	// The hub must not import room — room imports the hub — so the supplier is
	// injected here, where room and presence are already in scope, and before
	// the listener below accepts the first connection.
	home.SetSource(handlers.HomeDigest)

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

	// resolve (or mint) the visitor's session and attach the identity to the
	// request — the unified session system (arch/ACCOUNTS_AUTH_RATINGS.md)
	r.Use(auth.SessionMiddleware)

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

	// No /home/activity route. The home page's activity region is server-
	// rendered once and streamed over /socket/home from then on
	// (arch/HOME_ACTIVITY_STREAMING.md). There is deliberately no polling
	// fallback: the socket layer reconnects and re-snapshots on its own, so a
	// second live path would be a second thing to keep correct against a failure
	// mode the socket already handles.

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

	// the hands-on beginner's tutorial. Every lesson is its own URL so it can
	// be linked and resumed; an unknown slug redirects to the course start.
	// Registered here, well ahead of the /:id room wildcards, for the same
	// reason the other named pages are.
	r.Get("/learn", handlers.LearnHandler)
	r.Get("/learn/:slug", handlers.LearnLessonHandler)

	// paginated news feed page
	r.Get("/news", handlers.NewsHandler)

	// game database page handler
	r.Get("/db", handlers.DBHandler)

	// OpenGraph preview cards (the og:image targets scrapers fetch when a
	// octad.gg link is shared): the site-wide default card and the per-room
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

	// system console (site controls + active notices + audit feed) and the
	// moderation queue. Both 404 for anyone without the role; per-player actions
	// live on the player page instead.
	// The console is three pages, grouped by what an operator is doing. Each is
	// a real route so a tab is a URL, and so each page runs only its own reads.
	r.Get("/system", handlers.SystemHandler)
	r.Get("/system/people", handlers.SystemPeopleHandler)
	r.Get("/system/log", handlers.SystemLogHandler)
	// the audit feed on its own, for the filter form + pager's htmx swaps
	r.Get("/system/actions", handlers.SystemActionsHandler)
	// the instance panel on its own, for its self-poll (admin only)
	r.Get("/system/stats", handlers.SystemStatsHandler)
	r.Get("/moderation", handlers.ModerationHandler)

	// the public staff page: who holds the moderation tools. Open to everybody
	// — moderation here is not anonymous — and reachable from the footer.
	r.Get("/staff", handlers.StaffHandler)

	// public player pages. Registered before the room wildcards for the same
	// reason /game/:uuid is: "/@/drewtest" would otherwise match /:id/:num and
	// be read as room "@" game "drewtest". "@" cannot occur in a generated room
	// id, so ordering fully resolves the ambiguity.
	r.Get("/@/:username", handlers.ProfileHandler)

	// direct archived-game permalink by UUID (301s to the canonical
	// /<room_id>/<n> when the game has a room). Registered before the room
	// wildcards so "game" is never captured as a room id.
	r.Get("/game/:uuid", handlers.ArchiveGameByUUIDHandler)

	// archived-game data for in-room match browsing (immutable JSON; see
	// ArchiveGameJSONHandler)
	r.Get("/api/room/:id/game/:num", handlers.ArchiveGameJSONHandler)

	// cached per-ply engine evals for a finished game (uncacheable — the
	// background evaluator fills them lazily; see ArchiveGameEvalsHandler)
	r.Get("/api/room/:id/game/:num/evals", handlers.ArchiveGameEvalsHandler)

	// free-exploration analysis (archive pages + post-game bot analysis): the
	// study-style seam that applies/evaluates one explored position per
	// request. Rate-limited because a cache-missing eval is real engine CPU.
	r.Post("/api/analysis", middleware.AnalysisLimiter(), handlers.AnalysisHandler)

	// the /learn tutorial's move endpoint: applies and judges one lesson move.
	// Rate-limited on the same reasoning as analysis — the graduation game's
	// replies are real (if small) engine searches — but with a budget sized for
	// somebody working through a lesson rather than stepping a line.
	r.Post("/api/learn", middleware.LearnLimiter(), handlers.LearnAPIHandler)

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
