package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/www/handlers/api/account"
	"github.com/dechristopher/lio/www/handlers/api/mod"
	"github.com/dechristopher/lio/www/handlers/api/pools"
	"github.com/dechristopher/lio/www/handlers/api/report"
	"github.com/dechristopher/lio/www/handlers/api/stats"
	"github.com/dechristopher/lio/www/middleware"
)

// Wire up all the API handlers to the /api router
func Wire(a fiber.Router) {
	// GET /pools - retrieve rating pools JSON
	a.Get("/pools", pools.RatingPoolsHandler)

	// account/auth endpoints (register, login, logout, availability probe),
	// rate-limited per client IP on top of the keyed login limiter inside
	account.Wire(a.Group("/auth", middleware.AuthAPILimiter()))

	// site administration & moderation (arch/ADMIN_MODERATION.md). Rate-limited
	// like the auth group; every handler inside independently re-checks the
	// caller's role, so the group is organizational, not a security boundary.
	mod.Wire(a.Group("/mod", middleware.AuthAPILimiter()))

	// player reports — the one moderation surface that is not privileged. Rate
	// limited because it is reachable by any logged-in visitor, and a queue is
	// only useful while someone can still read it.
	report.Wire(a.Group("/report", middleware.AuthAPILimiter()))

	// statistics API group
	stat := a.Group("/stat")
	// GET /stat/site - retrieve site activity statistics
	stat.Get("/site", stats.SiteStatsHandler)
}
