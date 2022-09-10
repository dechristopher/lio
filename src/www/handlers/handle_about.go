package handlers

import (
	"github.com/dechristopher/lio/util"
	"github.com/gofiber/fiber/v2"
)

// AboutHandler executes the about page template
func AboutHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, 200, "about",
		"About", "main")
}

// AboutBoardHandler executes the about the board page template
func AboutBoardHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, 200, "about",
		"About", "board")
}

// AboutRulesHandler executes the about rules page template
func AboutRulesHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, 200, "about",
		"About", "rules")
}

// AboutMiscHandler executes the about misc page template
func AboutMiscHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, 200, "about",
		"About", "misc")
}
