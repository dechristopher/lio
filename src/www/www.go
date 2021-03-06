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

	viewsFs = util.PickFS(env.IsDev(), views, "./views")
	staticFs = util.PickFS(env.IsDev(), static, "./static")
	engine = html.NewFileSystem(viewsFs, ".html")

	// enable template engine reloading on dev
	engine.Reload(env.IsDev())

	r := fiber.New(fiber.Config{
		Prefork:               false,
		ServerHeader:          "lioctad.org",
		StrictRouting:         false,
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

	// sub-router with compression enabled
	sub := r.Group("/")

	// wire up all middleware components
	middleware.WireMiddleware(sub, staticFs)

	// home handler
	r.Get("/", handlers.IndexHandler)

	// JSON service health / status handler
	r.Get("/lio", handlers.StatusHandler)

	//r.Get("/*", func(c *fiber.Ctx) error {
	//	err := c.Status(200).SendFile("dist/index.html")
	//	if err != nil {
	//		return err
	//	}
	//	return nil
	//})

	// Custom 404 page
	middleware.NotFound(r)
}
