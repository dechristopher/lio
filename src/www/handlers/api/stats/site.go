package stats

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lioctad/game"
	"github.com/dechristopher/lioctad/www/ws"
)

// stats for active players and games on site
type stats struct {
	Players int `json:"p"`
	Games   int `json:"g"`
}

// SiteStatsHandler returns the number of players and
// games active on the site
func SiteStatsHandler(c *fiber.Ctx) error {
	var players int
	for _, sockMap := range ws.ChanMap {
		players += sockMap.Length()
	}

	return c.Status(200).JSON(stats{
		Players: players,
		Games:   len(game.Games),
	})
}
