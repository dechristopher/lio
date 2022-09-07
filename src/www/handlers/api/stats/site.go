package stats

import (
	"sync"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/room"
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lio/util"
)

const memDur = 10

// stats for active players and games on site
type stats struct {
	Players int `json:"p"`
	Games   int `json:"g"`
}

var memStats = stats{
	Players: 0,
	Games:   0,
}

var lastStats = time.Now()
var statLock = sync.Mutex{}

// SiteStatsHandler returns the number of players and
// games active on the site
func SiteStatsHandler(c *fiber.Ctx) error {
	if time.Since(lastStats) > (time.Second * time.Duration(memDur)) {
		locked := statLock.TryLock()
		if locked {
			util.DebugFlag("stat", "STAT", "pulling site stats")
			var players int
			channel.Map.Range(func(ch, _ any) bool {
				sockMap := channel.Map.GetSockMap(ch.(string))
				if sockMap != nil {
					players += sockMap.Length()
				}
				return true
			})
			memStats = stats{
				Players: players,
				Games:   room.Count(),
			}
			lastStats = time.Now()
			statLock.Unlock()
		}
	}

	return c.Status(200).JSON(memStats)
}
