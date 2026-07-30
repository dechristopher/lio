// Package presence answers "who is on the site right now" from the open
// WebSocket connections, which is the one place that knows.
//
// Every page holds exactly one socket, so the channel directory is the roster:
//
//	Room, in play         /socket/<room id>
//	Room, create and wait /socket/wait/<room id>
//	Home                  /socket/tv
//	All other pages       /socket/me
//
// One walk of that directory is therefore both the headcount and the named
// list — each socket carries the account it authenticated as
// (channel.Connected), so nothing here needs a query to name a member.
//
// This replaced an inference from HTTP request timestamps: the home page polled
// /home/activity every 5 seconds, and a viewer counted as present for 12
// seconds after their last poll. That reading was narrow and late. It could
// only ever see the home page, so a signed-in reader anywhere else on the site
// was invisible; it kept a departed viewer for up to a TTL after their tab
// closed; and it made presence a property of a poll interval rather than of a
// connection. The socket layer already holds the fact
// (arch/NOTIFICATIONS.md — "Presence from the notification channel").
//
// Seats are the one thing sockets cannot answer, so the caller supplies them:
// whether a connected person is at a board, or waiting in a challenge of their
// own, is room state. Presence intersects the two, so a seated player who has
// dropped their connection is not listed on the strength of a seat they are no
// longer holding open.
package presence

import (
	"sort"
	"strings"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
)

// Snapshot is the site-wide online picture, produced by a single walk so its
// parts can never disagree: Total is the headcount behind the "Online" stat
// tile, Members the named accounts within it (sorted, capped), and Anon the
// count holding no account. Members is capped for display but Anon is computed
// from the uncapped tally, so Total always equals Anon plus the number of
// distinct accounts online — capped or not.
type Snapshot struct {
	Total   int
	Members []message.OnlineMember
	Anon    int
}

// Online returns the site-wide presence picture: every session holding a live
// socket anywhere on the site, named from the account that socket
// authenticated as.
//
// seated maps each seated human's uid to their room identity — the same name,
// plus the Playing and Busy flags a socket cannot know (see room.HomeListing).
// It is an overlay, not a source: a uid that holds no socket is not online,
// whatever seat it still occupies.
//
// limit caps the named roster; a limit of zero or less asks for the counts
// only, for callers (the /system console) that render a headcount and no names.
//
// Presence counts *people*, not sessions. Anonymous visitors are only
// identifiable by uid, so each anonymous session is one person; but an account
// signed in on a laptop and a phone holds two uids and is still one player, so
// accounts fold together by username. Without that fold the same member would
// appear twice in the roster and twice in the headcount.
func Online(seated map[string]message.OnlineMember, limit int) Snapshot {
	// named accumulates one entry per distinct account (keyed by lowercased
	// username, which is the identity the unique index enforces); anon counts
	// the account-less sessions alongside it
	named := make(map[string]message.OnlineMember)
	anon := 0

	// add folds one presence record in, upgrading an account already present if
	// another of its sessions is in a busier state
	add := func(m message.OnlineMember) {
		if m.Username == "" {
			anon++
			return
		}
		key := strings.ToLower(m.Username)
		prev, seen := named[key]
		if !seen {
			named[key] = m
			return
		}
		// One account can hold several sessions — a laptop at a board and a phone
		// on the home page. The busier record wins for both flags: somebody
		// playing on one device is playing, and somebody seated anywhere cannot
		// take a challenge on another device either.
		if (m.Playing && !prev.Playing) || (m.Busy && !prev.Busy) {
			prev.Playing = prev.Playing || m.Playing
			prev.Busy = prev.Busy || m.Busy
			named[key] = prev
		}
	}

	for uid, acct := range channel.Connected() {
		// A seated session takes its record from the room, which carries the
		// flags. Everyone else — spectators, browsers, the creator of a room they
		// have wandered away from — is named from their socket.
		if m, isSeated := seated[uid]; isSeated {
			add(m)
			continue
		}
		add(message.OnlineMember{Username: acct.Name, Title: acct.Title})
	}

	snap := Snapshot{Total: len(named) + anon, Anon: anon}
	if limit <= 0 {
		return snap
	}
	snap.Members = make([]message.OnlineMember, 0, len(named))
	for _, m := range named {
		snap.Members = append(snap.Members, m)
	}
	// players at a board first, then alphabetical — a stable order, so the
	// 5s-polled roster does not reshuffle itself between refreshes
	sort.Slice(snap.Members, func(i, j int) bool {
		a, b := snap.Members[i], snap.Members[j]
		if a.Playing != b.Playing {
			return a.Playing
		}
		return strings.ToLower(a.Username) < strings.ToLower(b.Username)
	})
	if len(snap.Members) > limit {
		snap.Members = snap.Members[:limit]
	}
	return snap
}
