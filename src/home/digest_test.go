package home

import (
	"testing"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

func stats(playing, live, total int) *proto.HomeStats {
	return &proto.HomeStats{Playing: playing, Live: live, Total: total}
}

func players(names ...string) *proto.HomePlayers {
	p := &proto.HomePlayers{Online: make([]proto.HomePlayer, 0, len(names))}
	for _, n := range names {
		p.Online = append(p.Online, proto.HomePlayer{Name: n})
	}
	return p
}

// diffDigest sends only what moved. A frame that repeated the whole digest every
// second would undo most of the point of streaming it.
func TestDiffSendsOnlyChangedSections(t *testing.T) {
	prev := proto.HomePayload{
		Stats:      stats(2, 1, 40),
		Challenges: &proto.HomeChallenges{Items: []proto.HomeChallenge{{RoomID: "a"}}},
		Players:    players("nova"),
	}
	next := proto.HomePayload{
		Stats:      stats(3, 1, 40), // one more person online
		Challenges: &proto.HomeChallenges{Items: []proto.HomeChallenge{{RoomID: "a"}}},
		Players:    players("nova"),
	}

	got := diffDigest(&prev, &next)
	if got.Stats == nil || got.Stats.Playing != 3 {
		t.Fatalf("changed stats must be sent, got %#v", got.Stats)
	}
	if got.Challenges != nil {
		t.Errorf("unchanged challenges must not be sent, got %#v", got.Challenges)
	}
	if got.Players != nil {
		t.Errorf("unchanged players must not be sent, got %#v", got.Players)
	}
}

// An identical derive sends nothing at all, which is what keeps a quiet site
// quiet: the poll this replaced cost every viewer twelve requests a minute
// whether or not anything had happened.
func TestDiffOfIdenticalDigestIsEmpty(t *testing.T) {
	d := proto.HomePayload{
		Stats:      stats(1, 0, 7),
		Challenges: &proto.HomeChallenges{Items: nil},
		Players:    players("zed"),
	}
	same := proto.HomePayload{
		Stats:      stats(1, 0, 7),
		Challenges: &proto.HomeChallenges{Items: nil},
		Players:    players("zed"),
	}
	if got := diffDigest(&d, &same); !got.Empty() {
		t.Fatalf("identical derives must produce no frame, got %#v", got)
	}
}

// Emptying a section must be distinguishable from leaving it alone. This is the
// reason the sections are pointers: with omitempty alone, the last challenge
// being withdrawn would look identical to "challenges unchanged", and the
// withdrawn seek would stay on the page until something else changed.
func TestDiffReportsSectionBecomingEmpty(t *testing.T) {
	prev := proto.HomePayload{
		Challenges: &proto.HomeChallenges{Items: []proto.HomeChallenge{{RoomID: "a"}}},
	}
	next := proto.HomePayload{
		Challenges: &proto.HomeChallenges{Items: []proto.HomeChallenge{}},
	}
	got := diffDigest(&prev, &next)
	if got.Challenges == nil {
		t.Fatal("a section emptying must be sent, not treated as unchanged")
	}
	if len(got.Challenges.Items) != 0 {
		t.Errorf("emptied section must carry no items, got %#v", got.Challenges.Items)
	}
}

// The tick does no work when nothing has changed: no publish, no connection
// churn, and the backstop not yet due. This is the gate that makes an idle site
// cost nothing.
func TestTickSkipsDeriveWhenNothingChanged(t *testing.T) {
	h := newTestHub()
	calls := 0
	h.digest.src = func() (proto.HomePayload, Follows) {
		calls++
		return proto.HomePayload{Stats: stats(0, 0, 0)}, Follows{}
	}

	h.tick() // first tick always derives (derived is the zero time)
	if calls != 1 {
		t.Fatalf("first tick must derive, calls = %d", calls)
	}
	h.tick()
	h.tick()
	if calls != 1 {
		t.Fatalf("quiet ticks must not re-derive, calls = %d", calls)
	}

	// a published change reopens the gate
	h.digest.dirty = true
	h.tick()
	if calls != 2 {
		t.Fatalf("a dirty tick must re-derive, calls = %d", calls)
	}
}

// Presence moves without anything publishing — a socket opening anywhere on the
// site changes the roster and the headcount. The connection generation counter
// is what the tick notices that by.
func TestTickDerivesOnConnectionChange(t *testing.T) {
	h := newTestHub()
	calls := 0
	h.digest.src = func() (proto.HomePayload, Follows) {
		calls++
		return proto.HomePayload{Stats: stats(0, 0, 0)}, Follows{}
	}
	h.tick()
	if calls != 1 {
		t.Fatalf("first tick must derive, calls = %d", calls)
	}

	// track a socket on an unrelated channel: nothing publishes to the hub, but
	// the site-wide connection counter moves
	sm := channel.Map.GetSockMap("digest-gen-test")
	t.Cleanup(sm.Cleanup)
	sm.Track(channel.NewSocket(nil, "someone", "c1", "", channel.Account{ID: 9, Name: "nova"}))

	h.tick()
	if calls != 2 {
		t.Fatalf("a connection change must re-derive, calls = %d", calls)
	}
}

// The backstop derives even when neither signal fired. The signals cover
// everything known to move the digest, which is exactly why the backstop exists:
// "known to" is a claim about today's code, and a home page frozen until
// somebody reconnects would fail silently.
func TestTickDerivesOnBackstop(t *testing.T) {
	h := newTestHub()
	calls := 0
	h.digest.src = func() (proto.HomePayload, Follows) {
		calls++
		return proto.HomePayload{Stats: stats(0, 0, 0)}, Follows{}
	}
	h.tick()
	h.tick()
	if calls != 1 {
		t.Fatalf("quiet tick must not derive, calls = %d", calls)
	}

	// age the last derive past the backstop
	h.digest.derived = time.Now().Add(-digestBackstop - time.Second)
	h.tick()
	if calls != 2 {
		t.Fatalf("backstop must force a derive, calls = %d", calls)
	}
}

// A hub with no source injected must not panic or broadcast. This is the state
// during boot, before www wires the supplier.
func TestTickWithoutSourceIsInert(t *testing.T) {
	h := newTestHub()
	h.tick()
	if !h.digest.last.Empty() {
		t.Fatalf("a sourceless hub must hold no digest, got %#v", h.digest.last)
	}
}

// followSocket builds a tracked socket carrying a follow set, on a named
// channel. The nil connection is fine: Enqueue only touches the send buffer,
// which NewSocket allocates.
func followSocket(t *testing.T, chanName, connID string, acctID int64, follows ...int64) *channel.Socket {
	t.Helper()
	sm := channel.Map.GetSockMap(chanName)
	t.Cleanup(sm.Cleanup)
	s := channel.NewSocket(nil, "uid-"+connID, connID, "",
		channel.Account{ID: acctID, Name: "acct"})
	set := make(map[int64]struct{}, len(follows))
	for _, id := range follows {
		set[id] = struct{}{}
	}
	s.SetFollows(set)
	sm.Track(s)
	return s
}

// frames returns a counter reporting how many new frames landed on s since the
// previous call. A test socket has no writer goroutine draining its buffer, so
// the queue only ever grows and these deltas are exact.
func frames(s *channel.Socket) func() int {
	last := 0
	return func() int {
		n := s.Queued() - last
		last += n
		return n
	}
}

// The following badge goes to every socket on the site, not just the home
// page's. The header is on every page, so a reader sitting in a game must still
// learn that somebody they follow has signed on — the whole point of the fix.
func TestFollowBadgePushesToEveryChannel(t *testing.T) {
	h := newTestHub()
	inRoom := followSocket(t, "badge-room", "c1", 1, 42)
	onHome := followSocket(t, Channel, "c2", 2, 42)

	h.digest.src = func() (proto.HomePayload, Follows) {
		return proto.HomePayload{Stats: stats(0, 0, 0)}, Follows{
			Chips: func(map[int64]struct{}) []proto.HomePlayer { return nil },
			Count: func(f map[int64]struct{}) int { return len(f) },
		}
	}
	roomFrames, homeFrames := frames(inRoom), frames(onHome)
	h.tick()

	if roomFrames() == 0 {
		t.Error("a socket in a room must receive the badge count")
	}
	if homeFrames() == 0 {
		t.Error("a socket on the home page must receive the badge count")
	}
}

// A viewer whose count did not move is sent nothing. Without this the hub would
// wake every signed-in socket on the site once a second forever.
func TestFollowBadgeOnlySentOnChange(t *testing.T) {
	h := newTestHub()
	s := followSocket(t, "badge-quiet", "c1", 1, 42)

	count := 1
	h.digest.src = func() (proto.HomePayload, Follows) {
		return proto.HomePayload{Stats: stats(0, 0, 0)}, Follows{
			Count: func(map[int64]struct{}) int { return count },
		}
	}

	got := frames(s)
	h.tick()
	if n := got(); n != 1 {
		t.Fatalf("first tick must send the count once, got %d frames", n)
	}

	h.digest.dirty = true
	h.tick()
	if n := got(); n != 0 {
		t.Errorf("an unchanged count must send nothing, got %d frames", n)
	}

	// the last followed player leaves: zero is still a change, and it is the
	// frame that clears the dot
	count = 0
	h.digest.dirty = true
	h.tick()
	if n := got(); n != 1 {
		t.Errorf("a count dropping to zero must be sent, got %d frames", n)
	}
}

// An anonymous socket, or an account that follows nobody, is skipped entirely —
// there is no badge on those pages to keep true.
func TestFollowBadgeSkipsViewersWithNoFollows(t *testing.T) {
	h := newTestHub()
	anon := followSocket(t, "badge-anon", "c1", 0)

	h.digest.src = func() (proto.HomePayload, Follows) {
		return proto.HomePayload{Stats: stats(0, 0, 0)}, Follows{
			Count: func(map[int64]struct{}) int { return 3 },
		}
	}
	got := frames(anon)
	h.tick()
	if n := got(); n != 0 {
		t.Errorf("a socket with no follow set must be sent nothing, got %d frames", n)
	}
}
