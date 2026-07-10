package middleware

import (
	"io/fs"
	"os"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/favicon"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"

	"github.com/dechristopher/lio/assets"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/view"
)

// staticShortMaxAge (seconds) is the prod cache lifetime for static assets
// referenced by a literal, non-content-hashed URL — fonts, sounds, piece/board
// art, icons, manifest.json. Because their URL doesn't change when their bytes
// do, they must be able to go stale in bounded time (one hour) rather than the
// year that content-hashed assets safely get. See immutableIfHashed.
const staticShortMaxAge = 3600

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

	// Serve static files from /static preventing directory listings. Assets get a
	// split cache policy (see the assets package + immutableIfHashed):
	//   - content-hashed URLs (app.<hash>.css and the hashed JS/CSS emitted by
	//     view.asset): a changed file gets a new URL, so they are safely cached
	//     for a year, immutable — upgraded in ModifyResponse.
	//   - literal-URL assets (fonts, sounds, piece art, icons, manifest.json):
	//     the base MaxAge below, a short window, since their URL is stable.
	// In dev/local the hashes are stable across restarts, so send no Cache-Control
	// (MaxAge 0 => no header, and immutableIfHashed no-ops) to avoid the browser
	// holding a stale asset while iterating.
	staticMaxAge := 0
	if env.IsProd() {
		staticMaxAge = staticShortMaxAge
	}
	r.Use(static.New("", static.Config{
		FS:             strictFs{staticFS},
		MaxAge:         staticMaxAge,
		ModifyResponse: immutableIfHashed,
		// Skip the bare root "/": it is never a static asset (it is the home page),
		// and letting the fasthttp fileserver handle it as a directory request
		// clobbers the response headers/cookies that the upstream middleware
		// (security headers, CORS, user identity) already set — leaving the home
		// page with no CSP and, worse, no minted identity cookie. Every real static
		// path has a longer prefix and is unaffected; missing page paths (/about,
		// …) 404 cleanly through to their handlers.
		Next: func(c fiber.Ctx) bool {
			return c.Path() == "/"
		},
	}))
}

// immutableIfHashed upgrades the Cache-Control of a content-hashed asset to a
// one-year immutable cache. It runs (via the static middleware's ModifyResponse
// hook) only for successfully served files, after the base short-lived
// Cache-Control is set, and only rewrites it for manifest-known hashed URLs —
// whose bytes cannot change without changing the URL. Literal-URL assets keep
// the short window. No-op outside prod, where hashes are stable across restarts
// and assets are deliberately served uncached for local iteration.
func immutableIfHashed(c fiber.Ctx) error {
	if !env.IsProd() {
		return nil
	}
	name := strings.TrimPrefix(c.Path(), "/")
	if _, ok := assets.Real(name); ok {
		c.Set(fiber.HeaderCacheControl, "public, max-age=31536000, immutable")
	}
	return nil
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
