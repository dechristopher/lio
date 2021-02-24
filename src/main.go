package main

import (
	"embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html"
	"github.com/joho/godotenv"

	"github.com/dechristopher/lioctad/env"
	"github.com/dechristopher/lioctad/middleware"
	"github.com/dechristopher/lioctad/util"
)

var (
	port string

	//go:embed views/*
	views   embed.FS
	viewsFs http.FileSystem

	//go:embed static/*
	static   embed.FS
	staticFs http.FileSystem

	// fiber html template engine
	engine *html.Engine
)

// main does the thing
func main() {
	_ = godotenv.Load()
	port = os.Getenv("PORT")

	viewsFs = util.GetFS(env.IsDev(), views, "./views")
	staticFs = util.GetFS(env.IsDev(), static, "./static")
	engine = html.NewFileSystem(viewsFs, ".html")

	log.Printf("LIOCTAD.ORG - :%s - %s", port, env.GetEnv())

	// enable template engine reloading on dev
	engine.Reload(env.IsDev())

	r := fiber.New(fiber.Config{
		Prefork:               false,
		ServerHeader:          "lioctad.org",
		StrictRouting:         false,
		CaseSensitive:         true,
		ErrorHandler:          nil,
		DisableStartupMessage: env.IsProd(),
		Views:                 engine,
	})

	// wire up all middleware components
	middleware.WireMiddleware(r, staticFs)

	r.Get("/", homeHandler)

	// Custom 404 page
	middleware.NotFound(r)

	// Graceful shutdown with SIGINT
	// SIGTERM and others will hard kill
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		_ = <-c
		fmt.Println("LIOCTAD.ORG - shutting down")
		_ = r.Shutdown()
	}()

	if err := r.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Println(err)
	}

	// Exit cleanly
	log.Printf("LIOCTAD.ORG - exit")
	os.Exit(0)
}

// homeHandler executes the home page template
func homeHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "index",
		"Coming Soon", nil, 200)
}
