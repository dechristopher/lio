package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lio/view"
)

// renderAbout serves the about page for the given section, returning just the
// swappable content fragment for htmx tab requests and the full page otherwise.
func renderAbout(c *fiber.Ctx, section string) error {
	if view.IsHTMXFragment(c) {
		return view.Render(c, 200, view.AboutContent(section))
	}
	return view.Render(c, 200, view.About(view.PageMeta("About"), section))
}

// AboutHandler renders the about page
func AboutHandler(c *fiber.Ctx) error {
	return renderAbout(c, "main")
}

// AboutBoardHandler renders the about-the-board page
func AboutBoardHandler(c *fiber.Ctx) error {
	return renderAbout(c, "board")
}

// AboutRulesHandler renders the about-rules page
func AboutRulesHandler(c *fiber.Ctx) error {
	return renderAbout(c, "rules")
}

// AboutMiscHandler renders the about-misc page
func AboutMiscHandler(c *fiber.Ctx) error {
	return renderAbout(c, "misc")
}
