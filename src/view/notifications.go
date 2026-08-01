package view

import (
	"strconv"
	"strings"
)

// Helpers for the notification bell (arch/NOTIFICATIONS.md).

// NotifyBadgeLabel is the accessible name on the bell's badge. It states the
// count: a bare dot says something is waiting without saying whether that is
// one message or thirty, and the bell is the only place a screen reader can
// learn either.
//
// Exported because the client sets the same label when it repaints the badge on
// a page nobody reloads. The wording lives here, once, rather than in a second
// copy inside the JavaScript.
func NotifyBadgeLabel(n int64) string {
	if n == 1 {
		return "1 unread notification"
	}
	return strconv.FormatInt(n, 10) + " unread notifications"
}

// canChallenge reports whether v may send username a direct challenge
// (arch/NOTIFICATIONS.md Phase 2). It decides whether the sword renders at all.
//
// online means the player is holding a socket at this instant — the strict
// test, never the roster's window. A challenge is an invitation to play now, so
// it goes only to somebody who is here now; sent to anybody else it creates a
// room that waits for a person who is not coming, and a notification they read
// hours later. The roster lists people who have gone, and this is what keeps
// their chips from offering a control that cannot work.
//
// busy means the player is already seated somewhere — playing, or waiting in a
// challenge of their own. Those are the players who cannot take another game
// right now, and offering the control anyway would produce an invitation that
// sits unanswered until it expires.
//
// The rule is symmetric: a *viewer* who is seated cannot send one either
// (v.Seated). A challenge issued from the board you are sitting at would commit
// you to a second game you cannot play. This matters more than it used to now
// that the create-game dialog is mounted on every page — the control would
// otherwise be offered from the room page itself.
//
// The name comparison is case-insensitive because usernames are: the display
// case is not the identity, and a viewer must never be offered the chance to
// challenge themselves.
//
// None of this is a security boundary. The creation path re-resolves the target
// and refuses a self-challenge, an unknown name, a banned account, and one who
// is not online.
func canChallenge(v Viewer, username string, online, busy bool) bool {
	if !v.LoggedIn || !v.AccountsEnabled || !online || busy || v.Seated || username == "" {
		return false
	}
	return !strings.EqualFold(v.Username, username)
}
