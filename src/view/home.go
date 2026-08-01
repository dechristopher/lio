package view

import (
	"strconv"
	"strings"
	"time"

	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/pools"
	"github.com/dechristopher/lio/presence"
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

// shortSince renders how long ago somebody left as a chip-sized token ("4m").
// It is minute-grained because that is the whole range it ever sees: the roster
// covers presence.ActiveWindow, and anybody inside navGrace still reads as here,
// so the value runs from under a minute to a quarter of an hour.
//
// It is separate from shortAgo for the same reason that one is separate from
// RelativeDay: shortAgo speaks in hours and days, which is right for a
// registration date and says "0h" for everything this one has to render.
//
// Under a minute reads "now" rather than being rounded up to "1m". Rounding was
// the first attempt and it disagreed with the tooltip below, which called the
// same moment "just now" — the chip and its own title attribute contradicting
// each other. A word here is precedented: the arrival chips say "new".
func shortSince(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	m := int(time.Since(t).Minutes())
	if m < 1 {
		return "now"
	}
	return strconv.Itoa(m) + "m"
}

// leftPhrase is shortSince's prose form, for the chip's tooltip — the same
// split the arrival chips make, where the abbreviation carries the row and the
// sentence carries the title attribute. Keep the two agreeing about where a
// minute begins.
func leftPhrase(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	m := int(time.Since(t).Minutes())
	switch {
	case m < 1:
		return "left just now"
	case m == 1:
		return "left 1 minute ago"
	default:
		return "left " + strconv.Itoa(m) + " minutes ago"
	}
}

// hasPlayers reports whether the community panel has anything to show. An
// entirely empty panel renders nothing at all rather than a stack of empty
// states — a quiet site should look quiet, not broken.
func hasPlayers(c message.Community) bool {
	return len(c.Online) > 0 || len(c.Newest) > 0 || len(c.Following) > 0
}

// rosterNote is the roster's footnote. It carries the three things the chips
// above it cannot say for themselves, in one dim line:
//
//	the visitors who hold no account — how many, and when the viewer is one of
//	them, that they are among them. The quietest possible version of the account
//	pitch: it states a fact about visibility rather than asking for anything,
//	and the welcome card above does the actual asking.
//
//	how many named members the display cap left out, so a big evening reads as
//	a big evening rather than as exactly onlineShown people.
//
//	the window the list covers. This is the one that earns its place: the roster
//	holds people who are no longer connected, and without it a chip with no dot
//	and no challenge button is a puzzle. It is the same footnote the old forums
//	printed under their own lists, for the same reason.
//
// Empty when there is nothing to say, which renders no line. lio-home.js
// composes the same string for streamed frames — the viewer-dependent half
// cannot be broadcast — so the two must be changed together.
func rosterNote(c message.Community, loggedIn bool) string {
	parts := make([]string, 0, 3)
	if c.Anon == 1 {
		parts = append(parts, "1 anonymous visitor")
	} else if c.Anon > 1 {
		parts = append(parts, strconv.Itoa(c.Anon)+" anonymous visitors")
	}
	if c.Anon > 0 && !loggedIn {
		parts[0] += " (including you)"
	}
	if c.More > 0 {
		parts = append(parts, strconv.Itoa(c.More)+" more not shown")
	}
	// Only alongside an actual roster: with no chips above it there is no window
	// to explain, and the line would be a footnote to nothing.
	//
	// Either roster counts. A viewer whose only rows are people they follow —
	// which is every viewer whose follows are the only ones here — still needs
	// the line, because those chips carry the same undotted departed state that
	// the window is here to explain.
	if len(c.Online) > 0 || len(c.Following) > 0 {
		parts = append(parts, activeWindowNote)
	}
	return strings.Join(parts, " · ")
}

// activeWindowNote states the roster's window in words. It is derived from the
// constant itself so the sentence cannot outlive the behaviour it describes.
var activeWindowNote = "active in the last " +
	strconv.Itoa(int(presence.ActiveWindow.Minutes())) + " minutes"

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
