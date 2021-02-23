package middleware

import (
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/dechristopher/lioctad/env"
	"github.com/dechristopher/lioctad/util"
)

func WireMiddleware(r *fiber.App) {
	r.Use(requestid.New())

	// Compress responses
	r.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))

	r.Use(cors.New(cors.Config{
		AllowOrigins: corsOrigins(),
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// STDOUT request logger
	r.Use(logger.New(logger.Config{
		// For more options, see the Config section
		TimeZone:   "local",
		TimeFormat: "2006-01-02T15:04:05-0700",
		Format:     "${ip} ${header:x-forwarded-for} ${header:x-real-ip} [${time}] ${pid} ${locals:requestid} \"${method} ${path} ${protocol}\" ${status} ${latency} \"${referrer}\" \"${ua}\"\n",
		Output:     os.Stdout,
	}))

	//predefined route for favicon at root of domain
	r.Use(favicon.New(favicon.Config{
		File: "./static/res/ico/favicon.ico",
	}))

	// Serve static files from /static/res preventing directory listings
	r.Use(filesystem.New(filesystem.Config{
		Root:   strictFs{http.Dir("./static")},
		MaxAge: 86400,
	}))
}

// NotFound wires the final 404 handler after all other
// handlers are defined. Acts as the final fallback.
func NotFound(r *fiber.App) {
	r.Use(func(c *fiber.Ctx) error {
		return util.HandleTemplate(c, "404",
			"404", nil, 404)
	})
}

// corsOrigins returns the proper CORS origin configuration
// for the current environment
func corsOrigins() string {
	if env.IsProd() {
		return "https://lioctad.org"
	}
	return "https://localhost:4444, https://dev.lioctad.org"
}
