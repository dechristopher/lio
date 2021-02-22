package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/template/html"
	"github.com/joho/godotenv"
)

type env string

const (
	prod env = "prod"
	dev  env = "dev"
)

var (
	port   string
	engine = html.New("./views", ".html")
)

// main does the thing
func main() {
	_ = godotenv.Load()
	port = os.Getenv("PORT")

	log.Printf("LIOCTAD.ORG - :%s - %s", port, getEnv())

	// enable template reloading on dev
	engine.Reload(getEnv() == dev)

	r := fiber.New(fiber.Config{
		ServerHeader:          "lioctad.org",
		StrictRouting:         false,
		CaseSensitive:         true,
		ErrorHandler:          nil,
		DisableStartupMessage: false,
		Views:                 engine,
	})

	r.Get("/", homeHandler)

	//predefined route for favicon at root of domain
	r.Get("/favicon.ico", faviconHandler)

	// Serve static files from /static/res preventing directory listings
	r.Use(filesystem.New(filesystem.Config{
		Root:   strictFs{http.Dir("./static")},
		MaxAge: 86400,
	}))

	// Custom 404 page
	r.Use(func(c *fiber.Ctx) error {
		return handleTemplate(c, "404", "404", nil, 404)
	})

	// Graceful shutdown with SIGINT. SIGTERM and others will hard kill
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		_ = <-c
		fmt.Println("Gracefully shutting down...")
		_ = r.Shutdown()
	}()

	if err := r.Listen(fmt.Sprintf(":%s", port)); err != nil {
		log.Println(err)
	}

	// Exit cleanly
	log.Printf("LIOCTAD.ORG - shutdown")
	os.Exit(0)
}

// homeHandler executes the home page template
func homeHandler(c *fiber.Ctx) error {
	return handleTemplate(c, "index", "Coming Soon", nil, 200)
}

// faviconHandler returns the default favicon image
func faviconHandler(c *fiber.Ctx) error {
	return c.SendFile("./static/res/ico/favicon.ico", true)
}

func handleTemplate(c *fiber.Ctx, template string, name string, data interface{}, status int) error {
	return c.Status(status).Render(
		template,
		genPageModel(name, data),
		"layouts/main")
}

// getEnv returns the current environment
func getEnv() env {
	if os.Getenv("DEPLOY") == "prod" {
		return prod
	}
	return dev
}

// strictFs is a Custom strict filesystem implementation to
// prevent directory listings for resources
type strictFs struct {
	fs http.FileSystem
}

// Open only allows existing files to be pulled, not directories
func (sfs strictFs) Open(path string) (http.File, error) {
	f, err := sfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err == nil && s.IsDir() {
		index := strings.TrimSuffix(path, "/") + "/index.html"
		if _, err := sfs.fs.Open(index); err != nil {
			return nil, err
		}
	}
	return f, nil
}

// pageModel contains runtime information that
// can be used during page template rendering
type pageModel struct {
	Env      env
	PageName string
	Data     interface{}
}

// genPageModel generates the global page model
func genPageModel(name string, data interface{}) pageModel {
	return pageModel{
		Env:      getEnv(),
		PageName: name,
		Data:     data,
	}
}
