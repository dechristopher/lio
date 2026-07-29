// Package presence tracks site-wide "who's online right now" for visitors that
// hold no room connection. Browsers sitting on the home page open only the
// live-games TV stream (/socket/tv) — a read-only global channel that presence
// intentionally ignores (it is not a room and is not walked by HomeListing) —
// and otherwise poll /home/activity over HTTP every few seconds, so their
// presence is inferred from a recent request timestamp per user id.
//
// In-room presence (seated players and spectators) is already authoritative via
// the channel SockMaps, so this package only fills the home-page gap. Online
// unions the two sources by user id, so a single human is never double-counted
// whether they are polling the home page, sitting in a room, or both.
//
// Each entry carries the viewer's account identity when they have one, so the
// same walk that produces the headcount also produces the named roster the home
// page shows. Anonymous viewers are counted but never named.
package presence

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dechristopher/lio/message"
)

// ttl is how long after a user's last home-page request we still consider them
// present. It must comfortably exceed the home page's poll interval (5s) so a
// single dropped poll never blinks an otherwise-active viewer offline.
const ttl = 12 * time.Second

// poller is one home-page viewer: when they were last seen, plus their account
// identity when they hold one (zero-valued Member for an anonymous visitor).
type poller struct {
	seen   time.Time
	member message.OnlineMember
}

var (
	mu      sync.Mutex
	pollers = make(map[string]poller)
)

// Touch records that the given user id was just seen on the home page, along
// with their account identity (the zero OnlineMember for an anonymous visitor).
// Empty ids (a request before the user context cookie is established) are
// ignored.
func Touch(uid string, member message.OnlineMember) {
	if uid == "" {
		return
	}
	mu.Lock()
	pollers[uid] = poller{seen: time.Now(), member: member}
	mu.Unlock()
}

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

// Online returns the site-wide presence picture: the supplied set of in-room
// users (uid → their identity, zero-valued when anonymous) unioned with every
// home-page poller seen within the ttl window. limit caps the named roster; a
// limit of zero or less asks for the counts only, for callers (the /system
// console) that render a headcount and no names.
// Stale pollers are pruned as they are scanned, so reads double as the map's
// garbage collector and the home page (polled continuously by every active
// viewer) keeps it bounded without a separate sweeper.
//
// Presence counts *people*, not sessions. Anonymous visitors are only
// identifiable by uid, so each anonymous session is one person; but an account
// signed in on a laptop and a phone holds two uids and is still one player, so
// accounts fold together by username. Without that fold the same member would
// appear twice in the roster and twice in the headcount.
//
// A member seated in a room outranks the same member's home-page entry: the
// in-room record carries Playing, which is the more interesting of the two
// states and the one a roster should show.
func Online(inRoom map[string]message.OnlineMember, limit int) Snapshot {
	now := time.Now()

	mu.Lock()
	defer mu.Unlock()

	// named accumulates one entry per distinct account (keyed by lowercased
	// username, which is the identity the unique index enforces); anon counts
	// the account-less sessions alongside it
	named := make(map[string]message.OnlineMember, len(inRoom))
	anon := 0

	// add folds one presence record in, upgrading an account already present to
	// Playing if any of its sessions is seated
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
		if m.Playing && !prev.Playing {
			prev.Playing = true
			named[key] = prev
		}
	}

	for _, m := range inRoom {
		add(m)
	}

	for uid, p := range pollers {
		if now.Sub(p.seen) > ttl {
			delete(pollers, uid)
			continue
		}
		if _, alsoInRoom := inRoom[uid]; alsoInRoom {
			// same session, already counted from the room — and the room's copy
			// is the one carrying Playing
			continue
		}
		add(p.member)
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
