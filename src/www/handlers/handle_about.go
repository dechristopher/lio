package handlers

import (
	"github.com/dechristopher/lio/util"
	"github.com/gofiber/fiber/v2"
)

// AboutHandler executes the about page template
func AboutHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "about",
		"About", "main", 200)
}

// AboutBoardHandler executes the about the board page template
func AboutBoardHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "about",
		"About", "board", 200)
}

// AboutRulesHandler executes the about rules page template
func AboutRulesHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "about",
		"About", "rules", 200)
}

// AboutMiscHandler executes the about misc page template
func AboutMiscHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "about",
		"About", "misc", 200)
}
