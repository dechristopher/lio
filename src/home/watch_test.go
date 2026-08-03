package home

import (
	"strconv"
	"testing"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

// chanSeq keeps every test's sockets on a channel of its own. channel.Map is
// process-global, so two tests sharing a channel name would see each other's
// connections.
var chanSeq int

// watcher tracks one socket and returns the key a watch on it is registered
// under, plus the socket itself so a test can count what it was sent.
func watcher(t *testing.T, uid string) (watchKey, *channel.Socket) {
	t.Helper()
	chanSeq++
	name := "watch-test-" + strconv.Itoa(chanSeq)
	sm := channel.Map.GetSockMap(name)
	sock := channel.NewSocket(nil, uid, "c1", "", channel.Account{})
	sm.Track(sock)
	t.Cleanup(func() {
		sm.Cleanup()
		channel.ForgetDepartures()
	})
	return watchKey{channel: name, uid: uid}, sock
}

// seated builds a Start event for a game between two named accounts.
func seated(room, game, white, black string) Event {
	ev := start(room, game)
	ev.WhiteSeat = proto.TVSeat{Name: white}
	ev.BlackSeat = proto.TVSeat{Name: black}
	return ev
}

func TestWatchPushesCurrentStateOnRegister(t *testing.T) {
	h := newTestHub()
	h.handle(start("r1", "g1"))

	key, sock := watcher(t, "u1")
	h.applyWatch(&watchReq{key: key, roomID: "r1"})

	if sock.Queued() != 1 {
		t.Fatalf("registering a watch should push the room's current state, queued = %d",
			sock.Queued())
	}
	if h.watch.byKey[key] != "r1" {
		t.Fatalf("watch not registered: %#v", h.watch.byKey)
	}
}

// A watch is what makes an *unfeatured* room reach a viewer at all — that is
// the whole reason the hover card cannot ride the broadcast grid.
func TestWatchStreamsAnUnfeaturedRoom(t *testing.T) {
	h := newTestHub()
	for i := 0; i < Cap; i++ {
		h.handle(start("f"+strconv.Itoa(i), "g"))
	}
	h.handle(start("rPool", "gPool")) // over cap: tracked, never featured

	key, sock := watcher(t, "u1")
	h.applyWatch(&watchReq{key: key, roomID: "rPool"})
	before := sock.Queued()

	// the grid emits nothing for this room...
	if out := h.handle(Event{Kind: Move, RoomID: "rPool", GameID: "gPool"}); len(out) != 0 {
		t.Fatalf("an unfeatured room should emit no grid delta, got %#v", out)
	}
	// ...but the watcher is still told
	h.pushWatch("rPool")
	if sock.Queued() != before+1 {
		t.Fatalf("watcher should receive the move, queued %d -> %d", before, sock.Queued())
	}
}

func TestWatchReplacesThePreviousRoom(t *testing.T) {
	h := newTestHub()
	h.handle(start("r1", "g1"))
	h.handle(start("r2", "g2"))

	key, _ := watcher(t, "u1")
	h.applyWatch(&watchReq{key: key, roomID: "r1"})
	h.applyWatch(&watchReq{key: key, roomID: "r2"})

	if _, ok := h.watch.byRoom["r1"]; ok {
		t.Fatalf("moving a watch should leave the room it came from")
	}
	if h.watch.byKey[key] != "r2" {
		t.Fatalf("watch should now be on r2, got %q", h.watch.byKey[key])
	}
}

func TestEmptyRoomCancelsTheWatch(t *testing.T) {
	h := newTestHub()
	h.handle(start("r1", "g1"))

	key, _ := watcher(t, "u1")
	h.applyWatch(&watchReq{key: key, roomID: "r1"})
	h.applyWatch(&watchReq{key: key, roomID: ""})

	if len(h.watch.byKey) != 0 || len(h.watch.byRoom) != 0 {
		t.Fatalf("an empty room id should clear the watch entirely: %#v %#v",
			h.watch.byKey, h.watch.byRoom)
	}
}

// A closed room can never produce another event, so a watch on it would sit in
// the registry forever if the Gone frame did not also release it.
func TestClosedRoomReleasesItsWatchers(t *testing.T) {
	h := newTestHub()
	h.handle(start("r1", "g1"))

	key, sock := watcher(t, "u1")
	h.applyWatch(&watchReq{key: key, roomID: "r1"})
	before := sock.Queued()

	h.handle(Event{Kind: RoomClosed, RoomID: "r1"})
	h.pushWatch("r1")

	if sock.Queued() != before+1 {
		t.Fatalf("a closing room should push a final frame, queued %d -> %d",
			before, sock.Queued())
	}
	if len(h.watch.byKey) != 0 || len(h.watch.byRoom) != 0 {
		t.Fatalf("a closed room must release its watchers: %#v %#v",
			h.watch.byKey, h.watch.byRoom)
	}
}

// The registry cannot depend on a client saying goodbye — a page navigating away
// just closes its socket. Delivery is what discovers that.
func TestWatchIsPrunedWhenTheSocketIsGone(t *testing.T) {
	h := newTestHub()
	h.handle(start("r1", "g1"))

	key := watchKey{channel: "watch-test-gone", uid: "u-gone"}
	h.applyWatch(&watchReq{key: key, roomID: "r1"})

	// applyWatch's own push already found no sockets on that channel
	if len(h.watch.byKey) != 0 {
		t.Fatalf("a watch with no live socket must not be retained: %#v", h.watch.byKey)
	}
}

func TestWatchPayloadReportsAMissingRoomAsGone(t *testing.T) {
	h := newTestHub()
	p := h.watchPayload("nope")
	if !p.Gone || p.Game != nil {
		t.Fatalf("an unknown room should be reported Gone with no game, got %#v", p)
	}
}

func TestLiveGameForMatchesEitherSeatIgnoringCase(t *testing.T) {
	h := newTestHub()
	h.handle(seated("r1", "g1", "Alice", "Bob"))

	if g := h.liveGameFor("alice"); g == nil || g.RoomID != "r1" {
		t.Fatalf("white seat should match case-insensitively, got %#v", g)
	}
	if g := h.liveGameFor("BOB"); g == nil || g.RoomID != "r1" {
		t.Fatalf("black seat should match case-insensitively, got %#v", g)
	}
	if g := h.liveGameFor("carol"); g != nil {
		t.Fatalf("a player in no game should match nothing, got %#v", g)
	}
}

// A bot seat carries its difficulty persona's name. "Queen" is a difficulty, not
// an account, and must never resolve to a live game for an account of that name.
func TestLiveGameForIgnoresBotSeats(t *testing.T) {
	h := newTestHub()
	ev := start("r1", "g1")
	ev.WhiteSeat = proto.TVSeat{Name: "Queen", Bot: true}
	ev.BlackSeat = proto.TVSeat{Name: "Alice"}
	h.handle(ev)

	if g := h.liveGameFor("Queen"); g != nil {
		t.Fatalf("a bot seat must not answer for an account name, got %#v", g)
	}
	if g := h.liveGameFor("Alice"); g == nil {
		t.Fatalf("the human seat should still match")
	}
}

// An anonymous seat carries the literal "Anonymous". Two anonymous players are
// not the same person, and no account is named that either.
func TestLiveGameForIgnoresEmptySeatNames(t *testing.T) {
	h := newTestHub()
	ev := start("r1", "g1")
	ev.WhiteSeat = proto.TVSeat{}
	ev.BlackSeat = proto.TVSeat{}
	h.handle(ev)

	if g := h.liveGameFor(""); g != nil {
		t.Fatalf("an empty name must match no seat, got %#v", g)
	}
}

// The returned game is a copy: a caller holding it must not be able to mutate
// the hub's registry entry.
func TestLiveGameForReturnsACopy(t *testing.T) {
	h := newTestHub()
	h.handle(seated("r1", "g1", "Alice", "Bob"))

	g := h.liveGameFor("Alice")
	g.OFEN = "tampered"

	if h.games["r1"].OFEN == "tampered" {
		t.Fatalf("liveGameFor must not hand out the hub's own game")
	}
}
