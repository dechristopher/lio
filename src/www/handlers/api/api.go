package api

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lioctad/www/handlers/api/pools"
	"github.com/dechristopher/lioctad/www/handlers/api/stats"
)

// Wire up all of the API handlers to the /api router
func Wire(a fiber.Router) {
	// GET /pools - retrieve rating pools JSON
	a.Get("/pools", pools.RatingPoolsHandler)

	// statistics API group
	stat := a.Group("/stat")
	// GET /stat/site - retrieve site activity statistics
	stat.Get("/site", stats.SiteStatsHandler)
}
