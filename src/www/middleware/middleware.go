package middleware

import (
	"io/fs"
	"os"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/favicon"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/view"
)

const logFormatProd = "[${cookie:uid}] ${ip} ${reqHeader:x-forwarded-for} ${reqHeader:x-real-ip} " +
	"[${time}] ${pid} ${locals:requestid} \"${method} ${path} ${protocol}\" " +
	"${status} ${latency} \"${referer}\" \"${ua}\"\n"

const logFormatDev = "[${cookie:uid}] ${ip} [${time}] \"${method} ${path} ${protocol}\" " +
	"${status} ${latency}\n"

// Wire attaches all middleware to the given router
func Wire(r fiber.Router, staticFS fs.FS) {
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
		Stream:     os.Stdout,
	}))

	// Predefined route for favicon at root of domain
	r.Use(favicon.New(favicon.Config{
		File:       "res/ico/favicon.ico",
		FileSystem: staticFS,
	}))

	// Serve static files from /static preventing directory listings. Assets are
	// content-hashed (see the assets package), so a changed file gets a new URL —
	// making a 1-year cache safe in prod. In dev/local the hash is stable across
	// restarts, so send no Cache-Control (MaxAge 0 => no header) to avoid the
	// browser holding a stale asset while iterating.
	staticMaxAge := 0
	if env.IsProd() {
		staticMaxAge = 86400 * 365
	}
	r.Use(static.New("", static.Config{
		FS:     strictFs{staticFS},
		MaxAge: staticMaxAge,
	}))
}

// NotFound wires the final 404 handler after all other
// handlers are defined. Acts as the final fallback.
func NotFound(r *fiber.App) {
	r.Use(func(c fiber.Ctx) error {
		return view.Render(c, 404, view.NotFound(view.PageMeta("404")))
	})
}

func logFormat() string {
	if env.IsProd() {
		return logFormatProd
	}
	return logFormatDev
}
