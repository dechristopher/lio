package api

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/www/handlers/api/account"
	"github.com/dechristopher/lio/www/handlers/api/pools"
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

	// statistics API group
	stat := a.Group("/stat")
	// GET /stat/site - retrieve site activity statistics
	stat.Get("/site", stats.SiteStatsHandler)
}
