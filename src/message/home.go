package message

import "github.com/dechristopher/lio/variant"

// LiveGame is a lightweight snapshot of an in-progress room, shown in the
// home-page live-activity grid.
type LiveGame struct {
	RoomID  string
	Variant variant.Variant
	VsBot   bool
	Moves   int
}

// OpenChallenge is a snapshot of a human-vs-human room waiting for an
// opponent, shown as a joinable seek on the home page. Color is the side the
// creator chose; whoever joins takes the other. RaceTo is the room's match
// length (zero for a classic single game), surfaced so a joiner knows they are
// accepting a race-to match.
type OpenChallenge struct {
	RoomID  string
	Variant variant.Variant
	Color   string
	RaceTo  int
}

// SiteStats holds the live counters shown above the home-page activity feed.
type SiteStats struct {
	LiveGames      int
	OpenChallenges int
	// Playing is the site-wide "online now" count: distinct humans connected to
	// any room (seated players and spectators) unioned with recent home-page
	// viewers, deduped by user id so nobody is counted twice.
	Playing int
}
