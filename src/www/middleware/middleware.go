package middleware

import (
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
	"github.com/gofiber/fiber/v2/middleware/favicon"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/requestid"

	"github.com/dechristopher/lioctad/config"
	"github.com/dechristopher/lioctad/env"
	"github.com/dechristopher/lioctad/util"
)

const logFormatProd = "${ip} ${header:x-forwarded-for} ${header:x-real-ip} " +
	"[${time}] ${pid} ${locals:requestid} \"${method} ${path} ${protocol}\" " +
	"${status} ${latency} \"${referrer}\" \"${ua}\"\n"

const logFormatDev = "${ip} [${time}] \"${method} ${path} ${protocol}\" " +
	"${status} ${latency}\n"

// Wire attaches all middleware to the given router
func Wire(r fiber.Router, static http.FileSystem) {
	r.Use(requestid.New())

	// Compress responses
	r.Use(compress.New(compress.Config{
		Level: compress.LevelBestSpeed,
	}))

	// STDOUT request logger
	r.Use(logger.New(logger.Config{
		// For more options, see the Config section
		TimeZone:   "local",
		TimeFormat: "2006-01-02T15:04:05-0700",
		Format:     logFormat(),
		Output:     os.Stdout,
	}))

	// Predefined route for favicon at root of domain
	r.Use(favicon.New(favicon.Config{
		File:       "res/ico/favicon.ico",
		FileSystem: static,
	}))

	// Serve static files from /static preventing directory listings
	r.Use(filesystem.New(filesystem.Config{
		Root:   strictFs{static},
		MaxAge: 86400,
	}))

	// set browser id cookie
	// TODO rebuild this every time someone logs in
	r.Use(func(c *fiber.Ctx) error {
		if c.Cookies("bid") == "" {
			c.Cookie(&fiber.Cookie{
				Name:     "bid",
				Value:    config.GenerateCode(16, true),
				Path:     "/",
				Domain:   "",
				MaxAge:   0,
				Secure:   !env.IsLocal(),
				HTTPOnly: false,
				SameSite: "Strict",
			})
		}
		return c.Next()
	})
}

// NotFound wires the final 404 handler after all other
// handlers are defined. Acts as the final fallback.
func NotFound(r *fiber.App) {
	r.Use(func(c *fiber.Ctx) error {
		return util.HandleTemplate(c, "404",
			"404", nil, 404)
	})
}

func logFormat() string {
	if env.IsProd() {
		return logFormatProd
	}
	return logFormatDev
}
