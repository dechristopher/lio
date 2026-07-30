package message

import (
	"time"

	"github.com/dechristopher/lio/title"
	"github.com/dechristopher/lio/variant"
)

// LiveGame is a lightweight snapshot of an in-progress room, shown in the
// home-page live-activity grid.
type LiveGame struct {
	RoomID  string
	Variant variant.Variant
	VsBot   bool
	Moves   int
}

// OpenChallenge is a snapshot of a human-vs-human room waiting for an
// opponent, shown as a joinable seek on the home page. Color is the side a
// joiner would take (the still-open seat), so a browser sees the color they'd
// play — or "r" when the challenge is random-color, so they don't preemptively
// learn it. RaceTo is the room's match length (zero for a classic single game),
// surfaced so a joiner knows they are accepting a race-to match. Rated marks a
// members-only seek: anonymous visitors see it but must log in to join (an
// unrated "open" seek is joinable by anyone).
// CreatorName / CreatorTitle / CreatorRating are the challenger's account
// identity, so a seek reads as a person rather than a time control. All three
// are zero-valued for an anonymous creator, which the view renders as
// "Anonymous" — the visible difference between a named seek and a faceless one
// is itself the argument for holding an account. CreatorRating is only set for
// a rated challenge (it is captured at seat-claim, so surfacing it here costs
// no query).
type OpenChallenge struct {
	RoomID        string
	Variant       variant.Variant
	Color         string
	RaceTo        int
	Rated         bool
	CreatorName   string
	CreatorTitle  title.Title
	CreatorRating string
}

// SiteStats holds the live counters shown above the home-page activity feed.
type SiteStats struct {
	LiveGames      int
	OpenChallenges int
	// Playing is the site-wide "online now" count: distinct humans holding a
	// live socket anywhere on the site — at a board, watching one, waiting in a
	// challenge, or reading any other page — deduped so one person with several
	// tabs or devices counts once. Resolved by the presence package; zero as
	// HomeListing returns it.
	Playing int
	// TotalGames is the running count of finished games recorded to the archive
	// database (0 when Postgres is unconfigured).
	TotalGames int
}

// OnlineMember is one named account currently on the site. Anonymous visitors
// are deliberately not representable here: they are counted (SiteStats.Playing,
// Community.Anon) but never listed, because a roster of "Anonymous ×4" is noise
// and because being findable by name is exactly what an account buys.
type OnlineMember struct {
	Username string
	Title    title.Title
	// Playing marks a member seated in a live game rather than browsing, so the
	// roster can say who is actually at a board right now.
	Playing bool
	// Busy marks a member seated in any room — a live game, or a challenge of
	// their own that is still waiting for an opponent. It is what decides
	// whether they can be sent a direct challenge (arch/NOTIFICATIONS.md
	// Phase 2), and it is deliberately wider than Playing: somebody sitting on
	// their own waiting page is not playing, but they are already committed to
	// the next game they start.
	Busy bool
}

// NewMember is a recently registered account, shown so a visitor can see the
// player base growing and so new arrivals are discoverable on day one rather
// than only once they have a rating.
type NewMember struct {
	Username string
	Title    title.Title
	Joined   time.Time
}

// RatedMember is one row of the home-page leaderboard: a player's single best
// established rating, with the category it was earned in. One row per account —
// a player strong in three categories occupies one slot, not three.
type RatedMember struct {
	Username string
	Title    title.Title
	Category string
	Rating   int
	Games    int
}

// Community is the home page's people panel: who is here now, who just joined,
// and who is on top. Every field is independently optional — an empty section
// renders nothing rather than an empty-state placeholder, so a quiet site looks
// quiet rather than broken.
type Community struct {
	// Online lists named members currently on the site, capped for display;
	// Anon is how many of the total headcount hold no account.
	Online []OnlineMember
	Anon   int
	Newest []NewMember
	Top    []RatedMember
}
