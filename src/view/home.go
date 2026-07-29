package view

import (
	"strconv"
	"time"

	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/pools"
)

// View-side helpers for the home page's community panels.

// RelativeDay renders a coarse "N days ago", falling back to a date past a
// week. Day-grained on purpose: an exact time adds nothing to a games list or a
// new-member row, and publishes more than a public page should.
//
// It lives here rather than in the handlers because it is display formatting
// shared by four surfaces (the profile games list, the moderation log, the
// feedback/report queues, and the home page's new-member rows), and because the
// view package is already where the other public-page time formatting lives
// (see JoinedMonth).
func RelativeDay(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "just now"
	case d < 24*time.Hour:
		h := int(d.Hours())
		if h == 1 {
			return "1 hour ago"
		}
		return strconv.Itoa(h) + " hours ago"
	case d < 7*24*time.Hour:
		days := int(d.Hours() / 24)
		if days == 1 {
			return "yesterday"
		}
		return strconv.Itoa(days) + " days ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}

// shortAgo renders a join date as a chip-sized token ("new" / "6h" / "2d" /
// "Jul 20"). It is deliberately not RelativeDay: that one is prose for a list
// row ("2 days ago"), which is wider than the name it would sit beside in a
// chip. The chips carry the prose form in their tooltip, so the abbreviation
// costs nothing.
func shortAgo(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Hour:
		return "new"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	case d < 7*24*time.Hour:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	default:
		return t.Format("Jan 2")
	}
}

// hasPlayers reports whether the community panel has anything to show. An
// entirely empty panel renders nothing at all rather than a stack of empty
// states — a quiet site should look quiet, not broken.
func hasPlayers(c message.Community) bool {
	return len(c.Online) > 0 || len(c.Newest) > 0
}

// anonNote is the roster's footnote for the visitors who hold no account: how
// many there are, and — when the viewer is one of them — that they are among
// them. It is the quietest possible version of the account pitch: it states a
// fact about visibility rather than asking for anything, and the welcome card
// above does the actual asking.
//
// Empty when nobody is anonymous, which renders no line.
func anonNote(c message.Community, loggedIn bool) string {
	if c.Anon <= 0 {
		return ""
	}
	s := strconv.Itoa(c.Anon) + " anonymous"
	if c.Anon == 1 {
		s = "1 anonymous visitor"
	} else {
		s += " visitors"
	}
	if !loggedIn {
		s += " (including you)"
	}
	return s
}

// profileURL is a player page link for a username.
func profileURL(username string) string {
	return "/@/" + username
}

// ratingLine is a leaderboard row's rating plus the speed it was earned at
// ("1712 rapid"), so the number is never presented without saying what it
// measures.
//
// The stored category is a variant HTMLName ("three-five-rapid-deploy"), never
// something to print — pools.LookupRatingCategory is the single source of its
// display resolution, the same one the profile tiles go through. An unknown
// category (a legacy row whose variant is no longer curated) contributes no
// suffix rather than leaking the raw key.
func ratingLine(m message.RatedMember) string {
	s := strconv.Itoa(m.Rating)
	if info, ok := pools.LookupRatingCategory(m.Category); ok && info.Speed != "" {
		s += " " + info.Speed
	}
	return s
}
