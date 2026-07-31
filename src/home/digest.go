package home

import (
	"reflect"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

// The site activity digest (arch/HOME_ACTIVITY_STREAMING.md).
//
// This half of the hub replaced a per-viewer htmx poll of /home/activity. That
// poll re-derived the whole picture once per viewer per five seconds, and two
// of its steps walked the entire site: room.HomeListing takes every room's
// stateMu (the lock the move path needs), and presence.Online snapshots every
// SockMap. Since the viewers are most of the sockets, the second was O(V²).
//
// The digest is re-derived once per wakeup and shared, so the cost of keeping
// every home page current no longer grows with how many home pages there are.

const (
	// digestFloor is the shortest interval between two digest re-derives. It
	// bounds the work regardless of how much is happening: at most one walk and
	// one broadcast per second, at any audience size.
	//
	// The old poll's five seconds was never a freshness decision — it was the
	// largest cost that model could bear, paid 12·V times a minute. Now that the
	// walk is shared, a roster that reflects an arrival within a second is
	// affordable, and reads as live rather than as lagging.
	digestFloor = time.Second

	// digestBackstop forces a re-derive even when nothing signalled a change.
	//
	// The dirty signals below cover everything known to move the digest, which
	// is exactly why this exists: "known to" is a claim about today's code, and
	// the failure it protects against (a home page frozen until someone
	// reconnects) is silent. Ten seconds costs six walks a minute on a site
	// where nothing at all is happening.
	digestBackstop = 10 * time.Second
)

// Follows answers "which of the people online does this viewer follow", for one
// viewer, in the two shapes the two surfaces need.
//
// Both take the set of account ids that viewer follows, carried on their socket
// since connect (channel.Socket.Follows), and both close over the presence
// snapshot their DigestSource call built — so a viewer's Following section and
// the broadcast roster beside it describe the same instant.
//
// Chips builds the home page's Following section: whole roster rows, sorted.
// Count is just how many, for the header's following badge.
//
// Count is deliberately not len(Chips(…)). The badge goes to **every** socket on
// the site on every tick, and most of those are not home pages that will ever
// render a chip; making each of them sort and allocate a slice to learn a number
// would be the expensive half of a cheap question.
type Follows struct {
	Chips func(follows map[int64]struct{}) []proto.HomePlayer
	Count func(follows map[int64]struct{}) int
}

// DigestSource produces the current broadcast digest, plus the per-viewer
// lookup over the same snapshot.
//
// It is injected rather than imported: building one needs room and presence,
// and room imports this package (see the package comment).
//
// Returning both from one call is what keeps the walk single. The alternative —
// a second closure the hub calls per socket — would either re-walk presence for
// every viewer, which is the O(V²) the whole exercise removes, or quietly read
// a snapshot from a different instant than the roster it sits beside.
//
// The hub calls it from its own goroutine, never concurrently with itself.
type DigestSource func() (proto.HomePayload, Follows)

// SetSource injects the digest supplier. It is called once, from www, after
// room and presence are wired and before the listener accepts connections.
//
// Until it is called the hub streams the live-games grid and nothing else,
// which is the correct behaviour during boot: there is no digest to send yet
// because there is nothing on the site to describe.
func SetSource(digest DigestSource) {
	theHub.in <- hubMsg{sources: &sources{digest: digest}}
}

// MarkDirty tells the hub that something it renders may have changed. It never
// blocks and never allocates: the hub coalesces, so calling it on every seat
// claim and every room transition costs a non-blocking channel send.
func MarkDirty() {
	Publish(Event{Kind: dirtyOnly})
}

// sources carries the injected supplier over the hub's inbound channel, so it
// is installed by the owning goroutine like everything else rather than written
// under a lock.
type sources struct {
	digest DigestSource
}

// digestState is the hub's digest half. Like the grid registry it is touched
// only by run.
type digestState struct {
	src DigestSource

	// last is the digest as the viewers currently hold it. A fresh derive is
	// compared against it section by section, and only what differs is sent.
	// follows answers the per-viewer questions over the snapshot last was
	// built from.
	last    proto.HomePayload
	follows Follows

	// dirty records that something published a change since the last derive.
	// gen is the connection-change counter as of that derive: presence moves
	// without anything publishing, and comparing the counter is far cheaper
	// than the walk that would otherwise be needed to find out.
	dirty bool
	gen   uint64

	// derived is when the last full derive ran, for the backstop.
	derived time.Time

	// following is each home socket's last-sent Following section, and followed
	// each socket's last-sent online count, both keyed by connection id so a
	// viewer whose own answer did not change is sent nothing.
	//
	// followed is keyed across every channel, not just this one: the header's
	// following badge is on every page (see pushFollowing). Entries in both are
	// dropped when the socket goes away.
	following map[string][]proto.HomePlayer
	followed  map[string]int
}

// tick re-derives the digest if anything suggests it changed, broadcasts what
// differs, and updates each viewer's Following section.
//
// The re-derive is total rather than incremental, which is the whole design:
// the digest has many inputs (room lifecycle, seat claims, socket connect and
// disconnect, sign-in, the newest-member cache), and event-sourcing all of them
// would mean every one of those paths must remember to publish forever, with a
// missed publish showing up as a silently stale home page for everybody. A full
// derive is self-healing, which is the one virtue the poll had, and it is
// affordable precisely because it now happens once instead of once per viewer.
func (h *hub) tick() {
	d := &h.digest
	if d.src == nil {
		return
	}

	gen := channel.Generation()
	stale := time.Since(d.derived) >= digestBackstop
	if !d.dirty && gen == d.gen && !stale {
		// nothing published, nobody connected or disconnected, and the backstop
		// is not due: whatever the viewers hold is still correct
		return
	}
	d.dirty, d.gen, d.derived = false, gen, time.Now()

	next, follows := d.src()
	if p := diffDigest(&d.last, &next); !p.Empty() {
		h.broadcastHome(p)
	}
	d.last, d.follows = next, follows

	h.pushFollowing()
}

// diffDigest returns only the sections that differ between two derives. A nil
// section means "unchanged"; the client keeps what it has.
//
// The comparison is reflect.DeepEqual rather than hand-written per section. It
// is called once a second on a payload of a few dozen small structs, so its
// cost does not matter, and a hand-written comparison is one more thing that
// has to be updated when a field is added — the failure being a section that
// silently stops updating.
func diffDigest(prev, next *proto.HomePayload) proto.HomePayload {
	var out proto.HomePayload
	if !reflect.DeepEqual(prev.Stats, next.Stats) {
		out.Stats = next.Stats
	}
	if !reflect.DeepEqual(prev.Challenges, next.Challenges) {
		out.Challenges = next.Challenges
	}
	if !reflect.DeepEqual(prev.Players, next.Players) {
		out.Players = next.Players
	}
	return out
}

// pushFollowing tells each signed-in viewer about the people they follow who
// are online right now, and tells only the viewers whose own answer changed.
//
// Two surfaces, two scopes:
//
//   - The **header badge** is on every page, so the count walks every socket on
//     the site. A reader sitting in a game or on their profile must still learn
//     that somebody they follow has just signed on; before this the number was
//     only true at page paint and when the popover was opened, because presence
//     emitted nothing a pusher could hang off (the note this replaced in
//     view.Viewer said exactly that).
//   - The **home page's Following section** needs whole rows, so the chips go
//     only to sockets on this channel — nowhere else renders them.
//
// It is cheap for one reason: each socket's follow set was resolved once when it
// connected and is carried on it, so the question here is a handful of map
// lookups per connection rather than a query. The poll this replaced asked
// Postgres the same thing once per viewer per five seconds.
func (h *hub) pushFollowing() {
	d := &h.digest
	if d.follows.Count == nil {
		return
	}
	live := make(map[string]struct{})

	channel.EachSocket(func(chanName string, s *channel.Socket) {
		live[s.ID] = struct{}{}
		if len(s.Follows) == 0 {
			// an anonymous visitor, or an account that follows nobody: there is
			// nothing to say and nothing to remember
			return
		}

		// the badge, everywhere. A zero still goes out when it is a *change* —
		// that is the frame that clears a dot after the last followed player
		// leaves.
		n := d.follows.Count(s.Follows)
		if prev, sent := d.followed[s.ID]; !sent || prev != n {
			if d.followed == nil {
				d.followed = make(map[string]int)
			}
			d.followed[s.ID] = n
			s.Enqueue(proto.FollowOnlineMessage(n))
		}

		// the section, on the home page only
		if chanName != Channel || d.follows.Chips == nil {
			return
		}
		items := d.follows.Chips(s.Follows)
		if prev, sent := d.following[s.ID]; sent && reflect.DeepEqual(prev, items) {
			return
		}
		if d.following == nil {
			d.following = make(map[string][]proto.HomePlayer)
		}
		d.following[s.ID] = items
		p := proto.HomePayload{Following: &proto.HomeFollowing{Items: items}}
		s.Enqueue(p.Marshal())
	})

	// forget departed connections, or both maps grow for the life of the process
	for id := range d.following {
		if _, ok := live[id]; !ok {
			delete(d.following, id)
		}
	}
	for id := range d.followed {
		if _, ok := live[id]; !ok {
			delete(d.followed, id)
		}
	}
}

// connectDigest is the one-shot activity snapshot a newly connected viewer gets,
// alongside the grid snapshot: the whole digest as the hub last derived it, this
// socket's own identity, and its Following section.
//
// It is served from d.last rather than by re-deriving, so a burst of connects
// costs one walk at most (the next tick) instead of one each.
func (h *hub) connectDigest(s *channel.Socket) {
	d := &h.digest
	p := proto.HomePayload{
		Stats:      d.last.Stats,
		Challenges: d.last.Challenges,
		Players:    d.last.Players,
		Self: &proto.HomeSelf{
			Name: s.Acct.Name,
			// A viewer may send a challenge if they hold an account and are not
			// already committed to a room. Seat state is not on the socket, so
			// the room half of the view layer's canChallenge is left to the
			// server-rendered first paint; a home-page socket whose owner is
			// seated elsewhere is the rare case, and offering a sword that the
			// create-game endpoint then refuses is recoverable.
			Challenge: s.Acct.ID != 0,
		},
	}
	if d.follows.Chips != nil && len(s.Follows) > 0 {
		items := d.follows.Chips(s.Follows)
		p.Following = &proto.HomeFollowing{Items: items}
		if d.following == nil {
			d.following = make(map[string][]proto.HomePlayer)
		}
		d.following[s.ID] = items
	}
	s.Enqueue(p.Marshal())
}

// broadcastHome fans a digest delta out to every viewer on the channel.
func (h *hub) broadcastHome(p proto.HomePayload) {
	channel.Broadcast(p.Marshal(), channel.SocketContext{Channel: Channel})
}
