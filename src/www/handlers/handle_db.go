package handlers

import (
	"github.com/dechristopher/lio/util"
	"github.com/gofiber/fiber/v2"
)

// DBHandler executes the game database page template
func DBHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, 200, "db",
		"Game Database", nil)
}
