package api

import "github.com/gofiber/fiber/v2"

// Wire up all of the API handlers to the /api router
func Wire(a fiber.Router) {
	// GET /pools - retrieve rating pools JSON
	a.Get("/pools", RatingPoolsHandler)
}
