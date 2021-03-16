package www

import (
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html"
	"github.com/gofiber/websocket/v2"

	"github.com/dechristopher/lioctad/env"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/www/handlers"
	"github.com/dechristopher/lioctad/www/handlers/api"
	"github.com/dechristopher/lioctad/www/middleware"
	"github.com/dechristopher/lioctad/www/ws"
)

var (
	viewsFs  http.FileSystem
	staticFs http.FileSystem

	// fiber html template engine
	engine *html.Engine
)

// Serve all public endpoints
func Serve(views, static embed.FS) {
	util.Info(str.CMain, str.MInit, util.Version)

	// make filesystem location decision based on environment
	viewsFs = util.PickFS(env.IsLocal(), views, "./views")
	staticFs = util.PickFS(env.IsLocal(), static, "./static")
	// populate template engine from views filesystem
	engine = html.NewFileSystem(viewsFs, ".html")

	// enable template engine reloading on dev
	engine.Reload(env.IsLocal())

	r := fiber.New(fiber.Config{
		ServerHeader:          "lioctad.org " + util.Version,
		CaseSensitive:         true,
		ErrorHandler:          nil,
		DisableStartupMessage: true,
		Views:                 engine,
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
		env.GetEnv(), util.GetPort(), "none")

	// listen for connections on primary listening port
	if err := r.Listen(util.GetListenPort()); err != nil {
		log.Println(err)
	}

	// Exit cleanly
	util.Info(str.CMain, str.MExit)
	os.Exit(0)
}

// wireHandlers builds all of the websocket and http routes
// into the fiber app context
func wireHandlers(r *fiber.App, staticFs http.FileSystem) {
	// recover from panics
	r.Use(recover.New())

	// ws upgrade endpoint catch-all
	r.Use("/ws", ws.UpgradeHandler)

	// websocket connection listener
	r.Get("/ws/:chan", websocket.New(ws.ConnHandler))

	// sub-router with compression and other middleware enabled
	sub := r.Group("/")

	// wire up all middleware components
	middleware.Wire(sub, staticFs)

	// home handler
	// TODO not needed once we default SPAHandler
	r.Get("/", handlers.IndexHandler)

	// JSON service health / status handler
	r.Get("/lio", handlers.StatusHandler)

	// return static index.html for all other paths and
	// let React handle 404s so we get nice error pages
	//r.Get("/*", handlers.SPAHandlerInit(staticFs))

	// group for /api routes
	apiGroup := sub.Group("/api")

	// wire all of the api handlers
	api.Wire(apiGroup)

	// Custom 404 page
	// TODO not needed once we default SPAHandler
	middleware.NotFound(r)
}
