package handlers

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/view"
)

// DBHandler renders the game database page
func DBHandler(c fiber.Ctx) error {
	return view.Render(c, 200, view.DB(view.PageMeta("Game Database")))
}
