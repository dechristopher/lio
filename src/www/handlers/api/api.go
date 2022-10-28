package api

import (
	"github.com/dechristopher/lioctad/www/handlers"
	"github.com/dechristopher/lioctad/www/ws"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/websocket/v2"

	"github.com/dechristopher/lio/www/handlers/api/pools"
	"github.com/dechristopher/lio/www/handlers/api/stats"
)

// Wire up all the API handlers to the /api router
func Wire(a fiber.Router) {
	// websocket upgrade middleware
	a.Use("/socket", ws.UpgradeHandler)

	// websocket connection listener
	a.Get("/socket/:chan", websocket.New(ws.ConnHandler))
	// websocket
	a.Get("/socket/:type/:chan", websocket.New(ws.ConnHandler))

	// GET /pools - retrieve rating pools JSON
	a.Get("/pools", pools.RatingPoolsHandler)

	// statistics API group
	stat := a.Group("/stat")
	// GET /stat/site - retrieve site activity statistics
	stat.Get("/site", stats.SiteStatsHandler)

	// JSON service health / status handler
	a.Get("/lio", handlers.StatusHandler)

	// room handler
	a.Get("/:id", handlers.RoomHandler)

	// new room creation routes
	a.Post("/new/human", handlers.NewCustomRoomVsHuman)
	a.Get("/new/human/quick", handlers.NewQuickRoomVsHuman)
	a.Get("/new/computer", handlers.NewRoomVsComputer)
}
