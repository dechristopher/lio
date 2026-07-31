// Package presence answers "who is on the site right now" from the open
// WebSocket connections, which is the one place that knows.
//
// Every page holds exactly one socket, so the channel directory is the roster:
//
//	Room, in play         /socket/<room id>
//	Room, create and wait /socket/wait/<room id>
//	Home                  /socket/home
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
	"sync"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
)

// idsTTL bounds how stale the cached online-account set may be.
//
// The walk behind it snapshots every channel's socket map, which is cheap on
// the home page's 5-second poll and not cheap once per page render. The cache
// makes concurrent callers share one walk. Two seconds is under the home
// page's own poll interval, so no two surfaces can visibly disagree about who
// is online (arch/FOLLOWING.md).
const idsTTL = 2 * time.Second

var idsCache struct {
	sync.Mutex
	ids     []int64
	fetched time.Time
}

// OnlineIDs returns the account ids holding a live socket anywhere on the site,
// deduplicated — one entry for a person reading on a laptop and a phone.
// Anonymous sessions are absent: they hold no account, so nothing can be keyed
// to them.
//
// It is the left-hand side of the follow feature's central intersection. The
// caller hands this set to db.FollowedAmong, which answers which of them the
// viewer follows; the identities never leave the process (arch/FOLLOWING.md —
// "The graph answers membership").
func OnlineIDs() []int64 {
	idsCache.Lock()
	defer idsCache.Unlock()
	if time.Since(idsCache.fetched) < idsTTL {
		return idsCache.ids
	}
	seen := make(map[int64]struct{})
	for _, acct := range channel.Connected() {
		if acct.ID != 0 {
			seen[acct.ID] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	idsCache.ids = ids
	idsCache.fetched = time.Now()
	return ids
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
	// Accounts is every online member keyed by account id, uncapped and
	// unsorted — the same records Members is drawn from, before the display cap.
	//
	// It exists because a filtered view of the roster cannot be built from
	// Members: the follow feature has to be able to name somebody who is
	// fortieth in the site-wide order (arch/FOLLOWING.md). Exposing it is free,
	// since the fold below builds exactly this map on the way to Members.
	//
	// Callers hand its keys to db.FollowedAmong and map the answer back through
	// it, so the identities never leave the process.
	Accounts map[int64]message.OnlineMember
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
// its sessions fold together.
//
// The fold is on the **account id**, which is that identity directly and is
// already on every socket. It used to be on the lowercased username, which was
// a proxy for the same thing; keying on the id removes a string comparison from
// this walk and is what makes Snapshot.Accounts free to produce. A named record
// that somehow carries no id still folds by name rather than colliding with
// every other id-less record, which is what the second map below is for.
func Online(seated map[string]message.OnlineMember, limit int) Snapshot {
	// accounts holds one entry per distinct account id; byName is the fallback
	// for a named record that carries no id at all, which should not occur but
	// must not be allowed to collapse several people into one row if it ever
	// does. anon counts the account-less sessions alongside both.
	accounts := make(map[int64]message.OnlineMember)
	byName := make(map[string]message.OnlineMember)
	anon := 0

	conn := channel.Connected()

	// record resolves one session to the member it represents. A seated session
	// takes its record from the room, which carries the flags; everyone else —
	// spectators, browsers, the creator of a room they have wandered away from —
	// is named from their socket.
	record := func(uid string, acct channel.Account) message.OnlineMember {
		if m, isSeated := seated[uid]; isSeated {
			return m
		}
		return message.OnlineMember{ID: acct.ID, Username: acct.Name, Title: acct.Title}
	}

	// First pass: learn which account id each online *name* belongs to.
	//
	// This exists because the two sources do not have to agree about whether a
	// record carries an id — the socket always knows it, a hand-built seated
	// overlay might not. Without this, one person's two sessions could land in
	// two different buckets and be counted as two people, which is the exact
	// double-count the fold is here to prevent.
	nameID := make(map[string]int64)
	for uid, acct := range conn {
		m := record(uid, acct)
		if m.Username != "" && m.ID != 0 {
			nameID[strings.ToLower(m.Username)] = m.ID
		}
	}

	// merge decides what a second session of the same person contributes. One
	// account can hold several — a laptop at a board and a phone on the home
	// page. The busier record wins for both flags: somebody playing on one
	// device is playing, and somebody seated anywhere cannot take a challenge on
	// another device either.
	merge := func(prev, m message.OnlineMember) message.OnlineMember {
		prev.Playing = prev.Playing || m.Playing
		prev.Busy = prev.Busy || m.Busy
		return prev
	}

	for uid, acct := range conn {
		m := record(uid, acct)
		if m.Username == "" {
			anon++
			continue
		}
		key := strings.ToLower(m.Username)
		// borrow the id another of this person's sessions reported
		if m.ID == 0 {
			m.ID = nameID[key]
		}
		if m.ID != 0 {
			if prev, seen := accounts[m.ID]; seen {
				accounts[m.ID] = merge(prev, m)
			} else {
				accounts[m.ID] = m
			}
			continue
		}
		if prev, seen := byName[key]; seen {
			byName[key] = merge(prev, m)
		} else {
			byName[key] = m
		}
	}

	snap := Snapshot{
		Total:    len(accounts) + len(byName) + anon,
		Anon:     anon,
		Accounts: accounts,
	}
	if limit <= 0 {
		return snap
	}
	snap.Members = make([]message.OnlineMember, 0, len(accounts)+len(byName))
	for _, m := range accounts {
		snap.Members = append(snap.Members, m)
	}
	for _, m := range byName {
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
