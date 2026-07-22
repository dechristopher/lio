package account

import (
	"sort"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/pools"
)

// RatingsHandler returns the logged-in user's Glicko-2 ratings, one per time
// control (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5), lazy-loaded into the profile
// popover's ratings summary. Only categories the user has actually played appear;
// an empty list renders as "No rated games yet". Each row is resolved to its
// display parts (time control, speed group, game mode) so the client — which has
// no variant data — can render the labeled cards, sorted into a canonical order.
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
		Category    string `json:"category"`
		TimeControl string `json:"timeControl"`
		Speed       string `json:"speed"`
		Mode        string `json:"mode"`
		Rating      string `json:"rating"`
		Games       int    `json:"games"`
		order       int
	}
	out := make([]ratingRow, 0, len(list))
	for _, r := range list {
		info, known := pools.LookupRatingCategory(r.Category)
		if !known {
			// a legacy/unknown category no longer mapping to a curated variant:
			// skip it rather than render a bare, unlabeled key.
			continue
		}
		out = append(out, ratingRow{
			Category:    r.Category,
			TimeControl: info.TimeControl,
			Speed:       info.Speed,
			Mode:        info.Mode,
			Rating:      r.Rating.Display(),
			Games:       r.Rating.Games,
			order:       info.Order,
		})
	}
	// canonical order: default mode first, then bullet < blitz < 1+2 < 3+5.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Mode != out[j].Mode {
			return out[i].Mode < out[j].Mode // "" (default) sorts before named modes
		}
		return out[i].order < out[j].order
	})
	return c.JSON(fiber.Map{"ratings": out})
}
