// Package presence answers "who is around" from the open WebSocket
// connections, which is the one place that knows.
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
// # Two questions, not one
//
// The roster covers a window (ActiveWindow) rather than an instant, so this
// package answers two questions and never conflates them:
//
//	Online   holds a socket right now. Exact. Gates the presence dot and every
//	         challenge — an invitation must only reach somebody who is here.
//	Active   held one inside the window. What the home page lists, because a
//	         list that empties the moment a tab closes is a poor answer to
//	         "who is around" and reads as a dead site on a quiet evening.
//
// The window is deliberately NOT the TTL map this package was built to delete.
// That one inferred presence from polling, so its answer was a guess whose
// accuracy was capped by a poll interval. This one is driven by socket close
// events (channel.Departed): every row here is a connection the server watched
// open and watched close, and Online is still the exact instantaneous truth
// underneath. What the window adds is memory, not inference.
//
// Seats are the one thing sockets cannot answer, so the caller supplies them:
// whether a connected person is at a board, or waiting in a challenge of their
// own, is room state. A seat outlives the connection holding it, so a player
// whose socket dropped mid-game reads as at a board but not here — which is
// exactly what a spectator wants to know.
package presence

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
)

const (
	// ActiveWindow is how long somebody keeps a place in the roster after their
	// last socket closes. It is the old forums' "users active in the past N
	// minutes", and it exists for the same reason: a visitor arriving at a
	// quiet moment should see who has been around, not an empty card.
	//
	// Fifteen minutes is long enough to cover a player who stepped away between
	// games and short enough that the list still describes now.
	ActiveWindow = 15 * time.Minute

	// navGrace is how long a closed session keeps reading as fully Online.
	//
	// Every page holds its own socket, so following a link is a close followed
	// by an open. Without a grace period the digest tick can land in that gap
	// and the person's chip loses its dot, loses its challenge button and jumps
	// to the bottom of the list for a beat — on somebody else's screen, for no
	// reason they can see. channel.Track clears the departure stamp on the way
	// back in, so in the normal case this window is never reached at all; it
	// only has to outlast a page load.
	//
	// The cost is bounded and understood: a challenge can be sent up to this
	// long after somebody truly closes their last tab, and it waits for them as
	// a notification, which is what that panel is for.
	navGrace = 20 * time.Second

	// idsTTL bounds how stale the cached online-account set may be.
	//
	// The walk behind it snapshots every channel's socket map, which is cheap on
	// the home page's tick and not cheap once per page render. The cache makes
	// concurrent callers share one walk. Two seconds is under the home page's
	// own refresh interval, so no two surfaces can visibly disagree about who
	// is online (arch/FOLLOWING.md).
	idsTTL = 2 * time.Second
)

var idsCache struct {
	sync.Mutex
	ids     map[int64]struct{}
	fetched time.Time
}

// onlineSet returns the set of account ids holding a live socket, cached for
// idsTTL. Strictly Online — the roster window has no bearing on it.
func onlineSet() map[int64]struct{} {
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
	idsCache.ids = seen
	idsCache.fetched = time.Now()
	return seen
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
	set := onlineSet()
	ids := make([]int64, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	return ids
}

// AccountOnline reports whether an account holds a live socket right now.
//
// This is the strict test, not the window: it decides whether a profile page
// offers a challenge button, and a button that appears for somebody who left is
// a control that fails.
func AccountOnline(id int64) bool {
	if id == 0 {
		return false
	}
	_, ok := onlineSet()[id]
	return ok
}

// Snapshot is the site-wide picture, produced by a single walk so its parts can
// never disagree.
//
// Total is the headcount behind the home page's stat tile: everybody active
// inside ActiveWindow, named or not. Live is the same count taken strictly —
// sockets open at this instant — for the operator console, where "connected" is
// the question being asked and a window would blunt it.
//
// Members is the named roster, sorted and capped; More is how many named
// members the cap left out. Anon is computed from the uncapped tally, so Total
// always equals Anon plus the number of distinct accounts active — capped or
// not.
type Snapshot struct {
	Total   int
	Live    int
	Members []message.OnlineMember
	More    int
	Anon    int
	// Accounts is every active member keyed by account id, uncapped and
	// unsorted — the same records Members is drawn from, before the display cap.
	//
	// It exists because a filtered view of the roster cannot be built from
	// Members: the follow feature has to be able to name somebody who is
	// fortieth in the site-wide order (arch/FOLLOWING.md). Exposing it is free,
	// since the fold below builds exactly this map on the way to Members.
	//
	// Callers hand its keys to db.FollowedAmong and map the answer back through
	// it, so the identities never leave the process. Each record carries its own
	// Online flag, so a caller that wants only the live half — the follow
	// popover, the header badge — filters on that rather than on membership.
	Accounts map[int64]message.OnlineMember
}

// session is one uid's contribution to the fold: the identity behind it, and
// what kind of presence it is.
//
// online and live differ only inside navGrace, where a session that has just
// closed still reads as here. Everything a visitor sees keys off online; the
// operator console's strict headcount keys off live, because "connected" there
// means connected.
type session struct {
	acct   channel.Account
	online bool
	live   bool
	left   time.Time
}

// Online returns the site-wide picture: every session holding a live socket, or
// having held one inside ActiveWindow, named from the account that socket
// authenticated as.
//
// seated maps each seated human's uid to their room identity — the same name,
// plus the Playing and Busy flags a socket cannot know (see room.HomeListing).
// It is an overlay, not a source: a uid that has neither a socket nor a recent
// departure is not in the roster, whatever seat it still occupies.
//
// limit caps the named roster; a limit of zero or less asks for the counts
// only, for callers (the /system console) that render a headcount and no names.
//
// lastPlayed orders the roster within its tiers (see SortRoster) and may be
// nil, which falls back to the alphabetical tiebreak.
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
func Online(seated map[string]message.OnlineMember, limit int, lastPlayed map[int64]time.Time) Snapshot {
	// accounts holds one entry per distinct account id; byName is the fallback
	// for a named record that carries no id at all, which should not occur but
	// must not be allowed to collapse several people into one row if it ever
	// does. anon counts the account-less sessions alongside both.
	accounts := make(map[int64]message.OnlineMember)
	byName := make(map[string]message.OnlineMember)
	anon := 0

	// livePeople is the strict headcount, folded the same way the roster is so
	// the two cannot disagree about what one person is. Keying it by a string
	// lets one set cover all three identity cases below; the prefix keeps an
	// account id from ever colliding with a uid or a username.
	livePeople := make(map[string]struct{})

	sessions := activeSessions()

	// record resolves one session to the member it represents. A seated session
	// takes its identity from the room, which carries the flags; everyone else —
	// spectators, browsers, the creator of a room they have wandered away from —
	// is named from their socket.
	//
	// The presence fields are always this package's answer, never the overlay's:
	// the room knows about seats and nothing about connections, so a seated
	// record arrives with them zeroed.
	record := func(uid string, s session) message.OnlineMember {
		m, isSeated := seated[uid]
		if !isSeated {
			m = message.OnlineMember{ID: s.acct.ID, Username: s.acct.Name, Title: s.acct.Title}
		}
		m.Online = s.online
		if !s.online {
			m.Left = s.left
		}
		return m
	}

	// First pass: learn which account id each active *name* belongs to.
	//
	// This exists because the two sources do not have to agree about whether a
	// record carries an id — the socket always knows it, a hand-built seated
	// overlay might not. Without this, one person's two sessions could land in
	// two different buckets and be counted as two people, which is the exact
	// double-count the fold is here to prevent.
	nameID := make(map[string]int64)
	for uid, s := range sessions {
		m := record(uid, s)
		if m.Username != "" && m.ID != 0 {
			nameID[strings.ToLower(m.Username)] = m.ID
		}
	}

	// merge decides what a second session of the same person contributes. One
	// account can hold several — a laptop at a board and a phone on the home
	// page, or a closed tab alongside an open one. The busier record wins for
	// both flags: somebody playing on one device is playing, and somebody seated
	// anywhere cannot take a challenge on another device either.
	//
	// Online is a logical OR for the same reason, and it decides Left: a person
	// with any live session is here, and their earlier closed tab is not a
	// departure. Between two departures the later one wins, since that is when
	// this person actually left.
	merge := func(prev, m message.OnlineMember) message.OnlineMember {
		prev.Playing = prev.Playing || m.Playing
		prev.Busy = prev.Busy || m.Busy
		if m.Online {
			prev.Online = true
			prev.Left = time.Time{}
		} else if !prev.Online && m.Left.After(prev.Left) {
			prev.Left = m.Left
		}
		return prev
	}

	for uid, s := range sessions {
		m := record(uid, s)
		if m.Username == "" {
			anon++
			if s.live {
				// an anonymous visitor is only identifiable by uid, so that is
				// the person
				livePeople["uid:"+uid] = struct{}{}
			}
			continue
		}
		key := strings.ToLower(m.Username)
		// borrow the id another of this person's sessions reported
		if m.ID == 0 {
			m.ID = nameID[key]
		}
		if m.ID != 0 {
			if s.live {
				livePeople["id:"+strconv.FormatInt(m.ID, 10)] = struct{}{}
			}
			if prev, seen := accounts[m.ID]; seen {
				accounts[m.ID] = merge(prev, m)
			} else {
				accounts[m.ID] = m
			}
			continue
		}
		if s.live {
			livePeople["name:"+key] = struct{}{}
		}
		if prev, seen := byName[key]; seen {
			byName[key] = merge(prev, m)
		} else {
			byName[key] = m
		}
	}

	snap := Snapshot{
		Total:    len(accounts) + len(byName) + anon,
		Live:     len(livePeople),
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
	SortRoster(snap.Members, lastPlayed)
	if len(snap.Members) > limit {
		snap.More = len(snap.Members) - limit
		snap.Members = snap.Members[:limit]
	}
	return snap
}

// activeSessions reads both sources and folds them.
func activeSessions() map[string]session {
	return sessionsFrom(channel.Connected(), channel.Departed(ActiveWindow), time.Now())
}

// sessionsFrom folds the live directory and the remembered departures into one
// map keyed by uid, which is the shape both sources already use and the shape
// the seat overlay is keyed by.
//
// A uid in both is live: it has an open socket, and the stamp is from another
// tab it closed. A departure inside navGrace also counts as online — see the
// constant.
//
// It takes its inputs and its clock rather than reading them, so the window and
// the grace period can be tested for what they do at a given instant instead of
// by waiting out a real one.
func sessionsFrom(conn map[string]channel.Account, gone map[string]channel.Departure, now time.Time) map[string]session {
	out := make(map[string]session, len(conn)+len(gone))
	for uid, d := range gone {
		out[uid] = session{
			acct:   d.Account,
			online: now.Sub(d.At) <= navGrace,
			left:   d.At,
		}
	}
	for uid, acct := range conn {
		// the live record wins outright, and its account is the fresher of the
		// two: a session that signed in since it last closed a tab carries the
		// new identity here and the old one in the departure stamp
		out[uid] = session{acct: acct, online: true, live: true}
	}
	return out
}

// SortRoster orders a roster the way the question "who can I play right now" is
// asked, in four tiers:
//
//  1. here and free      — can accept a challenge this second
//  2. here and waiting   — seated in a challenge of their own
//  3. here and playing   — at a board
//  4. gone, within the window
//
// Within the first three, whoever played most recently reads first: that is
// what separates a player who has just finished a game from one who signed in
// and has been reading. The departed tier orders by when they left, most recent
// first — the person a visitor has only just missed.
//
// Ties break alphabetically, so a live region does not reshuffle itself between
// frames for no reason.
//
// lastPlayed may be nil (an unconfigured archive, or a caller that does not
// care), which leaves the alphabetical tiebreak to do the work.
//
// It is exported and shared by the site-wide roster and the viewer's Following
// section, because those are one roster split by whether the viewer cares about
// the person (arch/FOLLOWING.md) and an order that differed between them would
// read as two different lists.
func SortRoster(members []message.OnlineMember, lastPlayed map[int64]time.Time) {
	rank := func(m message.OnlineMember) int {
		switch {
		case !m.Online:
			return 3
		case m.Playing:
			return 2
		case m.Busy:
			return 1
		default:
			return 0
		}
	}
	sort.Slice(members, func(i, j int) bool {
		a, b := members[i], members[j]
		if ra, rb := rank(a), rank(b); ra != rb {
			return ra < rb
		}
		if !a.Online && !a.Left.Equal(b.Left) {
			return a.Left.After(b.Left)
		}
		if a.Online {
			pa, pb := lastPlayed[a.ID], lastPlayed[b.ID]
			if !pa.Equal(pb) {
				return pa.After(pb)
			}
		}
		return strings.ToLower(a.Username) < strings.ToLower(b.Username)
	})
}
