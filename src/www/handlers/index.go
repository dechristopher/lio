package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lioctad/util"
)

// indexHandler executes the home page template
func IndexHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "index",
		"Coming Soon", nil, 200)
}
