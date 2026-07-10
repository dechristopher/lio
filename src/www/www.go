package www

import (
	"embed"
	"io/fs"
	"log"
	"os"
	"os/signal"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/recover"

	"github.com/dechristopher/lio/assets"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/demo"
	"github.com/dechristopher/lio/env"
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

	r := fiber.New(fiber.Config{
		ServerHeader:  "lioctad.org " + config.Version,
		CaseSensitive: true,
		ErrorHandler:  nil,
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
		_ = r.Shutdown()
	}()

	util.Info(str.CMain, str.MStarted, util.TimeSinceBoot(),
		env.GetEnv(), config.GetPort(), "none")

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

	// JSON service health / status handler
	r.Get("/lio", handlers.StatusHandler)

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

	// new room creation routes. All POST (never GET): creating a room is a
	// state change, and a GET would be CSRF-able via a top-level cross-site
	// navigation (SameSite=Lax attaches cookies to those). The group is
	// rate-limited per client IP so a script can't spin up unbounded rooms.
	newRoom := r.Group("/new", middleware.RoomCreateLimiter())
	newRoom.Post("/game", handlers.NewCustomRoom)
	newRoom.Post("/human/quick", handlers.NewQuickRoomVsHuman)
	newRoom.Post("/computer", handlers.NewRoomVsComputer)

	// room handlers
	r.Get("/:id", handlers.RoomHandler)
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
