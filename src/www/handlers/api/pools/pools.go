package pools

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lioctad/pools"
)

// RatingPoolsHandler returns a list of all active rating
// pools, containing their time controls and their names
func RatingPoolsHandler(c *fiber.Ctx) error {
	return c.Status(200).JSON(pools.RatingPools)
}
