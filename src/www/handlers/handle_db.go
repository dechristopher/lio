package handlers

import (
	"github.com/dechristopher/lioctad/util"
	"github.com/gofiber/fiber/v2"
)

// DBHandler executes the game database page template
func DBHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "db",
		"Game Database", nil, 200)
}
