package handlers

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/demo"
)

// demoBatchSize is how many random demo games one /home/demo response carries.
// The home-page animator loops through them (one every ~15-40s of play) and
// refetches when it exhausts the batch, so a modest batch gives long, varied
// runs without a large payload.
const demoBatchSize = 12

// HomeDemoHandler returns a fresh batch of random Octad games as JSON for the
// home-page "What is Octad?" self-playing board (see demo.Batch and
// lio-home-demo.js). octadground can't generate moves client-side, so the games
// are produced server-side by the octad library and animated on the client.
func HomeDemoHandler(c fiber.Ctx) error {
	return c.JSON(demo.Batch(demoBatchSize))
}
