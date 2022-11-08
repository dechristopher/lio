package www

import (
	"embed"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/handlers"
	"github.com/dechristopher/lio/www/handlers/api"
	"github.com/dechristopher/lio/www/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/gofiber/template/html"
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
	//viewsFs = util.PickFS(env.IsLocal(), views, "./views")
	staticFs = util.PickFS(env.IsLocal(), static, "./static")
	// populate template engine from views filesystem
	//engine = html.NewFileSystem(viewsFs, ".html")

	// enable template engine reloading on dev
	//engine.Reload(env.IsLocal())

	//toUpperAny := func(s any) string {
	//	return strings.ToUpper(s.(string))
	//}

	// custom template rendering functions
	//engine.AddFuncMap(map[string]interface{}{
	//	"ToUpper":    strings.ToUpper,
	//	"ToUpperAny": toUpperAny,
	//	"ToLower":    strings.ToLower,
	//	"Title":      cases.Title(language.English).String,
	//})

	r := fiber.New(fiber.Config{
		ServerHeader:          "lioctad.org " + config.Version,
		CaseSensitive:         true,
		ErrorHandler:          nil,
		DisableStartupMessage: true,
		//Views:                 engine,
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

	// TODO remove because moved to api.go
	//// websocket upgrade middleware
	//r.Use("/socket", ws.UpgradeHandler)
	//// websocket connection listener
	//r.Get("/socket/:chan", websocket.New(ws.ConnHandler))
	//// websocket
	//r.Get("/socket/:type/:chan", websocket.New(ws.ConnHandler))

	// evaluate / set user context
	// TODO rebuild this every time someone logs in
	r.Use(user.ContextMiddleware)

	if env.IsProd() {
		// TODO update to serve the built React files
		//r.Static("/", "./views", fiber.Static{
		//	Index: "test.html",
		//})
	}

	// sub-router with compression and other middleware enabled
	sub := r.Group("/")

	// wire up all middleware components
	middleware.Wire(sub, staticFs)

	// group for /api routes
	apiGroup := sub.Group("/api")

	// wire all the api handlers
	api.Wire(apiGroup)

	// TODO remove because handled by the UI
	// home handler
	//r.Get("/", handlers.IndexHandler)

	// TODO remove because handled by the UI
	// other pages
	//r.Get("/about", handlers.AboutHandler)
	//r.Get("/about/board", handlers.AboutBoardHandler)
	//r.Get("/about/rules", handlers.AboutRulesHandler)
	//r.Get("/about/misc", handlers.AboutMiscHandler)

	// TODO remove because handled by the UI
	// game database page handler
	//r.Get("/db", handlers.DBHandler)

	// TODO remove because moved to api.go
	//// JSON service health / status handler
	//r.Get("/lio", handlers.StatusHandler)
	//// room handler
	//r.Get("/:id", handlers.RoomHandler)
	//// new room creation routes
	//r.Post("/new/human", handlers.NewCustomRoomVsHuman)
	//r.Get("/new/human/quick", handlers.NewQuickRoomVsHuman)
	//r.Get("/new/computer", handlers.NewRoomVsComputer)

	// room handlers
	r.Get("/:id", handlers.RoomHandler)
	r.Post("/:id/join", handlers.RoomJoinHandler)
	r.Post("/:id/cancel", handlers.RoomCancelHandler)

	// return static index.html for all other paths and let
	// React handle 404s so that we get nice error pages
	//r.Get("/*", handlers.SPAHandlerInit(staticFs))

	// TODO remove because handled by the UI
	// Custom 404 page
	//middleware.NotFound(r)
}
