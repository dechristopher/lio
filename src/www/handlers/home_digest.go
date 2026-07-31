package handlers

import (
	"sort"
	"strings"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/home"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/presence"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/view"
	"github.com/dechristopher/lio/www/ws/proto"
)

// The home activity digest's source (arch/HOME_ACTIVITY_STREAMING.md).
//
// The hub cannot build this itself: it needs room and presence, and room
// imports the hub. So the hub takes a closure, and this is it — the same idiom
// channel/handlers/crowd.go uses to let the room supply seats to a package the
// room already imports.
//
// This runs once per hub tick for the whole site. The htmx poll it replaced ran
// the same two walks once per viewer per five seconds, which made the presence
// walk O(V²) and had every home viewer taking every live room's stateMu twelve
// times a minute.

// HomeDigest derives the broadcast digest and the per-viewer follow lookup from
// a single presence snapshot.
//
// Both come from one call because both must describe the same instant: a
// Following section resolved against a newer snapshot than the roster beside it
// can show a player in one and not the other, which reads as a duplicate or a
// ghost. It is also the only way to keep the walk single — a lookup that
// re-derived presence per viewer would restore exactly the cost this removes.
func HomeDigest() (proto.HomePayload, home.Follows) {
	_, challenges, stats, seated := room.HomeListing()

	online := presence.Online(seated, onlineShown)
	stats.Playing = online.Total
	stats.TotalGames = int(db.TotalGames())

	// The broadcast roster is the raw capped list: it still contains the viewer
	// and the people they follow, because it is one payload for everybody and
	// the server does not know who is reading it. The client removes both
	// before rendering — see lio-home.js.
	//
	// Top is absent deliberately. The leaderboard sits outside the activity
	// region (view.Index renders leaderboardCard beside it, not within it), so
	// streaming it would push a payload nothing displays.
	digest := view.HomeDigest(challenges, stats, message.Community{
		Online: online.Members,
		Anon:   online.Anon,
		Newest: arrivalsWithPresence(db.NewestMembers(), online),
	})

	// Close over the uncapped Accounts map rather than the capped Members list.
	// That is the whole reason this section can be correct: the roster shows the
	// eight most prominent accounts online, and a followed player who ranks
	// fortieth still belongs in the viewer's own list (arch/FOLLOWING.md).
	accounts := online.Accounts

	// eachFollowed calls fn for every followed account that is online, iterating
	// the smaller side. A viewer may follow up to MaxFollowing accounts while a
	// handful are online, or follow three while the site is busy; walking the
	// shorter map keeps this bounded by the better of the two in both
	// directions.
	eachFollowed := func(follows map[int64]struct{}, fn func(message.OnlineMember)) {
		if len(follows) == 0 || len(accounts) == 0 {
			return
		}
		if len(follows) <= len(accounts) {
			for id := range follows {
				if m, ok := accounts[id]; ok {
					fn(m)
				}
			}
			return
		}
		for id, m := range accounts {
			if _, ok := follows[id]; ok {
				fn(m)
			}
		}
	}

	return digest, home.Follows{
		// Chips: whole rows for the home page's Following section.
		Chips: func(follows map[int64]struct{}) []proto.HomePlayer {
			out := make([]message.OnlineMember, 0, min(len(follows), len(accounts)))
			eachFollowed(follows, func(m message.OnlineMember) {
				out = append(out, m)
			})
			sortAvailableFirst(out)
			return view.HomePlayers(out)
		},
		// Count: the header badge, asked of every socket on the site every
		// tick — so it allocates nothing and sorts nothing.
		Count: func(follows map[int64]struct{}) int {
			n := 0
			eachFollowed(follows, func(message.OnlineMember) { n++ })
			return n
		},
	}
}

// arrivalsWithPresence marks the recent arrivals who are on the site right now,
// so a new player reads as somebody you can actually challenge rather than only
// as a name and a date.
//
// It **copies** rather than stamping in place. db.NewestMembers serves a
// process-wide TTL cache, and the slice it hands back is shared by every caller
// and every read until the cache expires; writing presence into those elements
// would pin one moment's dots to every later read — a green dot on somebody who
// left minutes ago, which is worse than no dot at all.
//
// Presence is keyed on the account id rather than the username. The id is what
// the snapshot folds on, and the site moved off name-keying deliberately (see
// presence.Online) — matching by name here would reintroduce the case-folding
// question that fold exists to avoid.
func arrivalsWithPresence(newest []message.NewMember, online presence.Snapshot) []message.NewMember {
	if len(newest) == 0 {
		return nil
	}
	out := make([]message.NewMember, len(newest))
	copy(out, newest)
	if len(online.Accounts) == 0 {
		return out
	}
	for i := range out {
		m, here := online.Accounts[out[i].ID]
		if !here {
			continue
		}
		out[i].Online = true
		out[i].Playing = m.Playing
		out[i].Busy = m.Busy
	}
	return out
}

// sortAvailableFirst orders a follow section: people who can play right now
// first, then alphabetically.
//
// This section answers "who can I play right now", so the answer leads. Ties
// break alphabetically so a live region does not reshuffle itself between
// frames for no reason — the same requirement the polled version had, for the
// same reason.
func sortAvailableFirst(members []message.OnlineMember) {
	sort.Slice(members, func(i, j int) bool {
		a, b := members[i], members[j]
		if a.Busy != b.Busy {
			return !a.Busy
		}
		return strings.ToLower(a.Username) < strings.ToLower(b.Username)
	})
}
