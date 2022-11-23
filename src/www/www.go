package www

import (
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/handlers"
	"github.com/dechristopher/lio/www/handlers/api"
	"github.com/dechristopher/lio/www/middleware"
	"github.com/dechristopher/lio/www/ws"
)

var (
	viewsFs  http.FileSystem
	staticFs http.FileSystem

	// fiber html template engine
	engine *html.Engine
)

// Serve all public endpoints
func Serve(views, static embed.FS) {
	util.Info(str.CMain, str.MInit, config.Version)

	// make filesystem location decision based on environment
	viewsFs = util.PickFS(env.IsLocal(), views, "./views")
	staticFs = util.PickFS(env.IsLocal(), static, "./static")
	// populate template engine from views filesystem
	engine = html.NewFileSystem(viewsFs, ".html")

	// enable template engine reloading on dev
	engine.Reload(env.IsLocal())

	toUpperAny := func(s any) string {
		return strings.ToUpper(s.(string))
	}

	// custom template rendering functions
	engine.AddFuncMap(map[string]interface{}{
		"ToUpper":    strings.ToUpper,
		"ToUpperAny": toUpperAny,
		"ToLower":    strings.ToLower,
		"Title":      cases.Title(language.English).String,
	})

	r := fiber.New(fiber.Config{
		ServerHeader:          "lioctad.org " + config.Version,
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
		env.GetEnv(), config.GetPort(), "none")

	// listen for connections on primary listening port
	if err := r.Listen(config.GetListenPort()); err != nil {
		log.Println(err)
	}

	// Exit cleanly
	util.Info(str.CMain, str.MExit)
	os.Exit(0)
}

// wireHandlers builds all the websocket and http routes
// into the fiber app context
func wireHandlers(r *fiber.App, staticFs http.FileSystem) {
	// recover from panics
	r.Use(recover.New())

	// Configure CORS
	r.Use(cors.New(cors.Config{
		AllowOrigins: config.CorsOrigins(),
		AllowHeaders: "Origin, Content-Type, Accept",
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
	sub.Get("/lio", handlers.StatusHandler)

	// group for /api routes
	apiGroup := sub.Group("/api")

	// wire all the api handlers
	api.Wire(apiGroup)

	// home handler
	// TODO not needed once we default SPAHandler
	sub.Get("/", handlers.IndexHandler)

	// other pages
	sub.Get("/about", handlers.AboutHandler)
	sub.Get("/about/board", handlers.AboutBoardHandler)
	sub.Get("/about/rules", handlers.AboutRulesHandler)
	sub.Get("/about/misc", handlers.AboutMiscHandler)

	// game database page handler
	sub.Get("/db", handlers.DBHandler)

	// new room creation routes
	sub.Post("/new/human", handlers.NewCustomRoomVsHuman)
	sub.Get("/new/human/quick", handlers.NewQuickRoomVsHuman)
	sub.Get("/new/computer", handlers.NewRoomVsComputer)

	// room handlers
	sub.Get("/:id", handlers.RoomHandler)
	sub.Post("/:id/join", handlers.RoomJoinHandler)
	sub.Post("/:id/cancel", handlers.RoomCancelHandler)

	// return static index.html for all other paths and let
	// React handle 404s so that we get nice error pages
	//r.Get("/*", handlers.SPAHandlerInit(staticFs))

	// Custom 404 page
	// TODO not needed once we default SPAHandler
	middleware.NotFound(r)
}
