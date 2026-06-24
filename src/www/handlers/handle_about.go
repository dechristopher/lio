package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lio/view"
)

// AboutHandler renders the about page
func AboutHandler(c *fiber.Ctx) error {
	return view.Render(c, 200, view.About(view.PageMeta("About"), "main"))
}

// AboutBoardHandler renders the about-the-board page
func AboutBoardHandler(c *fiber.Ctx) error {
	return view.Render(c, 200, view.About(view.PageMeta("About"), "board"))
}

// AboutRulesHandler renders the about-rules page
func AboutRulesHandler(c *fiber.Ctx) error {
	return view.Render(c, 200, view.About(view.PageMeta("About"), "rules"))
}

// AboutMiscHandler renders the about-misc page
func AboutMiscHandler(c *fiber.Ctx) error {
	return view.Render(c, 200, view.About(view.PageMeta("About"), "misc"))
}
