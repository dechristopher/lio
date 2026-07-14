package handlers

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/presence"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/view"
)

// IndexHandler renders the home page
func IndexHandler(c fiber.Ctx) error {
	presence.Touch(user.GetID(c))
	challenges, stats := homeActivity()

	meta := view.PageMeta("Free Online Octad")
	// one-shot notice for clients redirected off a room that no longer exists
	// (the ws layer sends them to /?notice=room-gone — typically an open
	// challenge dropped by a server restart, which doesn't persist waiting rooms)
	if c.Query("notice") == "room-gone" {
		meta.Notice = "That room is gone — it was most likely cleared by a " +
			"server update before the game started. Create a new game below."
	}

	return view.Render(c, 200, view.Index(meta, challenges, stats))
}

// HomeActivityHandler renders the live home-activity fragment (site stats, open
// challenges) polled by htmx from the home page. The live-games grid is no
// longer part of this fragment — it streams over /socket/tv (see tvWidget).
func HomeActivityHandler(c fiber.Ctx) error {
	presence.Touch(user.GetID(c))
	// live fragment: must never be served from the browser cache, or htmx's
	// self-poll swaps in a stale (pre-rebuild) copy of the stats/challenges
	c.Set("Cache-Control", "no-store")
	challenges, stats := homeActivity()
	return view.Render(c, 200, view.HomeActivity(challenges, stats))
}

// homeActivity gathers the shared home-page activity data and resolves the
// site-wide "online now" count by unioning the in-room humans with the recent
// home-page viewers (the calling handler having just Touch'd itself into the
// latter). The live-games slice from HomeListing is unused here now (the TV
// widget streams that), but stats.LiveGames still reflects the live count.
func homeActivity() ([]message.OpenChallenge, message.SiteStats) {
	_, challenges, stats, present := room.HomeListing()
	stats.Playing = presence.Online(present)
	stats.TotalGames = int(db.TotalGames())
	return challenges, stats
}
