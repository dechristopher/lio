package account

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
)

// RatingsHandler returns the logged-in user's Glicko-2 ratings per category
// (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5), lazy-loaded into the profile
// popover's ratings summary. Only categories the user has actually played
// appear; an empty list renders as "No rated games yet".
func RatingsHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	list, err := db.ListRatingsForUser(*sess.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not load ratings"})
	}
	type ratingRow struct {
		Category string `json:"category"`
		Rating   string `json:"rating"`
		Games    int    `json:"games"`
	}
	out := make([]ratingRow, 0, len(list))
	for _, r := range list {
		out = append(out, ratingRow{
			Category: r.Category,
			Rating:   r.Rating.Display(),
			Games:    r.Rating.Games,
		})
	}
	return c.JSON(fiber.Map{"ratings": out})
}
