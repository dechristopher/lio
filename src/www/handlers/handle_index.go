package handlers

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/presence"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/view"
)

// IndexHandler renders the home page
func IndexHandler(c fiber.Ctx) error {
	challenges, stats, community := homeActivity(c)

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

// There is no HomeActivityHandler any more. The activity region used to poll
// this package every five seconds per viewer; it is now server-rendered once by
// IndexHandler above and streamed over /socket/home from then on
// (arch/HOME_ACTIVITY_STREAMING.md — HomeDigest in home_digest.go is the hub's
// source).

// onlineShown bounds the named roster on the home page's players panel. The
// remainder is not hidden — everyone online is still in the headcount, and the
// panel says how many more there are.
const onlineShown = 8

// homeActivity gathers the home page's first paint and resolves the site-wide
// presence picture from the open sockets, with the room registry's seats
// overlaid so a connected player reads as playing or waiting. The live-games
// slice from HomeListing is unused here (the TV widget streams that), but
// stats.LiveGames still reflects the live count.
//
// This runs once per page load now, not once per viewer per five seconds. It
// still resolves the viewer's Following section over HTTP, because the first
// paint has no socket yet — the socket takes over the moment it opens (see
// HomeDigest).
//
// The newest/top panels come from TTL-cached database reads, so they cost a
// mutex rather than a query.
func homeActivity(c fiber.Ctx) ([]message.OpenChallenge, message.SiteStats, message.Community) {
	_, challenges, stats, seated := room.HomeListing()

	online := presence.Online(seated, onlineShown)
	stats.Playing = online.Total
	stats.TotalGames = int(db.TotalGames())

	var viewerID int64
	if acct := user.GetAccount(c); acct != nil {
		viewerID = acct.ID
	}
	followed := followedOnline(viewerID, online)
	return challenges, stats, message.Community{
		Online:    rosterFor(online.Members, followed, viewerID),
		Anon:      online.Anon,
		Following: followed,
		Newest:    arrivalsWithPresence(db.NewestMembers(), online),
		Top:       db.TopRated(),
	}
}

// followedOnline picks the viewer's own followed players out of the presence
// snapshot, available ones first (arch/FOLLOWING.md).
//
// This is the feature's central intersection, and the reason it is cheap: the
// snapshot already names everybody online, so the only question for the
// database is which of those ids the viewer follows — one index-only probe
// whose cost is bounded by how many people are on the site, not by how many
// accounts the viewer follows.
//
// It reads from Accounts rather than Members deliberately. Members is capped
// for display, and a followed player who is fortieth in the site-wide order
// still belongs in this section.
//
// A signed-out visitor follows nobody and costs no query at all.
func followedOnline(viewerID int64, online presence.Snapshot) []message.OnlineMember {
	if viewerID == 0 || len(online.Accounts) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(online.Accounts))
	for id := range online.Accounts {
		ids = append(ids, id)
	}
	followed, err := db.FollowedAmong(viewerID, ids)
	if err != nil {
		// The roster below is still worth rendering; this section is an extra
		// view of it, not a replacement for it.
		util.Error(str.CDB, "followed-online lookup failed error=%s", err.Error())
		return nil
	}

	out := make([]message.OnlineMember, 0, len(followed))
	for id := range followed {
		// the viewer's own row is never here: a self-follow cannot exist
		if m, ok := online.Accounts[id]; ok {
			out = append(out, m)
		}
	}
	sortAvailableFirst(out)
	return out
}

// rosterFor trims the site-wide roster to the people this section is actually
// for: everybody online except the viewer themselves, and except anybody
// already named in the Following section above it.
//
// The two follow sections are one roster split by whether the viewer cares
// about the person, so a name appearing in both would read as two people.
// Removing rather than marking also means the general roster keeps showing new
// faces instead of spending its eight slots on people the viewer already
// follows.
//
// The viewer goes too. They know they are here, their own chip is the one row
// in the list with nothing to do — canChallenge never offers a sword against
// yourself — and the section is for finding somebody else. They stay in the
// headcount beside it, which counts the site rather than the list.
func rosterFor(all, followed []message.OnlineMember, viewerID int64) []message.OnlineMember {
	if len(followed) == 0 && viewerID == 0 {
		return all
	}
	skip := make(map[int64]struct{}, len(followed)+1)
	for _, m := range followed {
		skip[m.ID] = struct{}{}
	}
	if viewerID != 0 {
		skip[viewerID] = struct{}{}
	}
	out := make([]message.OnlineMember, 0, len(all))
	for _, m := range all {
		if _, hit := skip[m.ID]; !hit {
			out = append(out, m)
		}
	}
	return out
}
