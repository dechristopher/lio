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
	touch(c)
	challenges, stats, community := homeActivity()

	meta := view.PageMeta("Free Online Octad")
	// one-shot notice for clients redirected off a room that no longer exists
	// (the ws layer sends them to /?notice=room-gone — typically an open
	// challenge dropped by a server restart, which doesn't persist waiting rooms)
	switch c.Query("notice") {
	case "room-gone":
		meta.Notice = "That room is gone — it was most likely cleared by a " +
			"server update before the game started. Create a new game below."
	case "maintenance":
		// refused by the maintenance switch (arch/ADMIN_MODERATION.md Phase 3).
		// The site-wide banner usually says more about why; this explains the
		// specific action that just didn't happen.
		meta.Notice = "New games are paused for maintenance right now. " +
			"Games already in progress are unaffected — please try again shortly."
	case "challenge-declined":
		// the invited player turned it down, and the room closed under the
		// challenger (arch/NOTIFICATIONS.md Phase 2). Said plainly: this is an
		// answer, not a fault, and the generic room-gone copy would read as one.
		meta.Notice = "Your challenge was declined. Create another game below, " +
			"or challenge somebody else from the players list."
	case "challenge-failed":
		// the invite could not be resolved — an unknown or banned account, or a
		// tampered form. Deliberately vague about which: a message that
		// distinguishes them would report whether an account exists.
		meta.Notice = "That challenge could not be sent. The player may no longer " +
			"be available — try again from their profile or the players list."
	}

	return view.Render(c, 200, view.Index(meta, challenges, stats, community))
}

// touch records the requesting viewer as present on the home page, carrying
// their account identity so the online roster can name them. An anonymous
// viewer is touched with the zero member: counted, never listed.
func touch(c fiber.Ctx) {
	var member message.OnlineMember
	if uc := user.GetContext(c); uc != nil && uc.Account != nil {
		member = message.OnlineMember{
			Username: uc.Account.Username,
			Title:    uc.Account.Title,
		}
	}
	presence.Touch(user.GetID(c), member)
}

// HomeActivityHandler renders the live home-activity fragment (site stats, open
// challenges) polled by htmx from the home page. The live-games grid is no
// longer part of this fragment — it streams over /socket/tv (see tvWidget).
func HomeActivityHandler(c fiber.Ctx) error {
	touch(c)
	// live fragment: must never be served from the browser cache, or htmx's
	// self-poll swaps in a stale (pre-rebuild) copy of the stats/challenges
	c.Set("Cache-Control", "no-store")
	challenges, stats, community := homeActivity()
	return view.Render(c, 200, view.HomeActivity(challenges, stats, community))
}

// onlineShown bounds the named roster on the home page's players panel. The
// remainder is not hidden — everyone online is still in the headcount, and the
// panel says how many more there are.
const onlineShown = 8

// homeActivity gathers the shared home-page activity data and resolves the
// site-wide presence picture by unioning the in-room humans with the recent
// home-page viewers (the calling handler having just touch'd itself into the
// latter). The live-games slice from HomeListing is unused here now (the TV
// widget streams that), but stats.LiveGames still reflects the live count.
//
// The newest/top panels come from TTL-cached database reads, so serving them on
// this path — which every viewer re-runs every 5 seconds — costs a mutex rather
// than a query.
func homeActivity() ([]message.OpenChallenge, message.SiteStats, message.Community) {
	_, challenges, stats, present := room.HomeListing()

	online := presence.Online(present, onlineShown)
	stats.Playing = online.Total
	stats.TotalGames = int(db.TotalGames())

	return challenges, stats, message.Community{
		Online: online.Members,
		Anon:   online.Anon,
		Newest: db.NewestMembers(),
		Top:    db.TopRated(),
	}
}
