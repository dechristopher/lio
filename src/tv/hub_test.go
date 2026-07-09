package tv

import (
	"fmt"
	"testing"

	"github.com/dechristopher/lio/www/ws/proto"
)

// newTestHub builds a hub with no inbound channel; tests drive handle directly.
func newTestHub() *hub {
	return &hub{
		games:    make(map[string]*proto.TVGame),
		featured: make([]string, 0, Cap),
	}
}

func start(room, game string) Event {
	return Event{Kind: Start, RoomID: room, GameID: game, OFEN: "ppkn/4/4/NKPP w NCFncf - 0 1"}
}

func TestFeaturedFillsToCapThenPools(t *testing.T) {
	h := newTestHub()

	// the first Cap rooms each claim a slot and emit a single Add
	for i := 0; i < Cap; i++ {
		room := fmt.Sprintf("r%d", i)
		out := h.handle(start(room, "g"+room))
		if len(out) != 1 || out[0].Add == nil || out[0].Add.RoomID != room {
			t.Fatalf("room %s: expected one Add delta, got %#v", room, out)
		}
	}
	if len(h.featured) != Cap {
		t.Fatalf("featured = %d, want %d", len(h.featured), Cap)
	}

	// an over-cap room is tracked but waits in the pool (no delta)
	out := h.handle(start("rOver", "gOver"))
	if len(out) != 0 {
		t.Fatalf("over-cap room should not emit a delta, got %#v", out)
	}
	if _, ok := h.games["rOver"]; !ok {
		t.Fatalf("over-cap room should still be tracked in the pool")
	}
	if h.featuredIndex("rOver") >= 0 {
		t.Fatalf("over-cap room should not be featured")
	}
}

func TestClosedFeaturedRoomBackfillsFromPool(t *testing.T) {
	h := newTestHub()
	for i := 0; i < Cap; i++ {
		h.handle(start(fmt.Sprintf("r%d", i), "g"))
	}
	h.handle(start("rPool", "gPool")) // waits in the pool

	out := h.handle(Event{Kind: RoomClosed, RoomID: "r0"})

	// expect a Remove for the closed room followed by an Add backfilling the slot
	if len(out) != 2 {
		t.Fatalf("expected Remove+Add, got %#v", out)
	}
	if out[0].Remove != "r0" {
		t.Fatalf("first delta should remove r0, got %#v", out[0])
	}
	if out[1].Add == nil || out[1].Add.RoomID != "rPool" {
		t.Fatalf("second delta should add rPool, got %#v", out[1])
	}
	if _, ok := h.games["r0"]; ok {
		t.Fatalf("closed room should be dropped from games")
	}
	if h.featuredIndex("r0") >= 0 {
		t.Fatalf("closed room should no longer be featured")
	}
	if h.featuredIndex("rPool") < 0 {
		t.Fatalf("backfilled room should now be featured")
	}
	if len(h.featured) != Cap {
		t.Fatalf("featured should remain full after backfill, got %d", len(h.featured))
	}
}

func TestClosedFeaturedRoomNoPoolJustShrinks(t *testing.T) {
	h := newTestHub()
	h.handle(start("a", "g1"))
	h.handle(start("b", "g2"))

	out := h.handle(Event{Kind: RoomClosed, RoomID: "a"})
	if len(out) != 1 || out[0].Remove != "a" {
		t.Fatalf("expected a single Remove for a, got %#v", out)
	}
	if len(h.featured) != 1 || h.featured[0] != "b" {
		t.Fatalf("featured should be just [b], got %#v", h.featured)
	}
}

func TestRematchKeepsSlot(t *testing.T) {
	h := newTestHub()
	h.handle(start("room", "game1"))
	before := append([]string(nil), h.featured...)

	// a rematch is a new Start for the same room with a new game id
	out := h.handle(start("room", "game2"))
	if len(out) != 1 || out[0].Add == nil || out[0].Add.GameID != "game2" {
		t.Fatalf("rematch should emit an Add carrying the new game id, got %#v", out)
	}
	if fmt.Sprint(h.featured) != fmt.Sprint(before) {
		t.Fatalf("rematch should not change slot order: %v -> %v", before, h.featured)
	}
	if g := h.games["room"]; g == nil || g.GameID != "game2" {
		t.Fatalf("registry should hold the new game id, got %#v", h.games["room"])
	}
}

func TestMoveAndEndDeltasOnlyForFeatured(t *testing.T) {
	h := newTestHub()
	// fill the grid, then add one pooled room
	for i := 0; i < Cap; i++ {
		h.handle(start(fmt.Sprintf("r%d", i), "g"))
	}
	h.handle(start("rPool", "gPool"))

	// a move on a featured room emits a Move delta
	out := h.handle(Event{Kind: Move, RoomID: "r0", GameID: "g", OFEN: "x", White: 100, Black: 90})
	if len(out) != 1 || out[0].Move == nil || out[0].Move.RoomID != "r0" {
		t.Fatalf("featured move should emit a Move delta, got %#v", out)
	}

	// a move on a pooled (non-featured) room emits nothing but updates state
	out = h.handle(Event{Kind: Move, RoomID: "rPool", GameID: "gPool", OFEN: "y"})
	if len(out) != 0 {
		t.Fatalf("pooled move should emit no delta, got %#v", out)
	}
	if g := h.games["rPool"]; g == nil || g.OFEN != "y" {
		t.Fatalf("pooled room state should still update, got %#v", h.games["rPool"])
	}

	// End on a featured room emits a Move delta flagged Over
	out = h.handle(Event{Kind: End, RoomID: "r0", GameID: "g", OFEN: "z"})
	if len(out) != 1 || out[0].Move == nil || !out[0].Move.Over {
		t.Fatalf("featured end should emit a Move delta with Over set, got %#v", out)
	}
}

func TestCrowdPatchesCountAndDeltasOnlyForFeatured(t *testing.T) {
	h := newTestHub()
	for i := 0; i < Cap; i++ {
		h.handle(start(fmt.Sprintf("r%d", i), "g"))
	}
	h.handle(start("rPool", "gPool"))

	// a count change on a featured room emits a count-only Crowd delta and
	// patches the registry (so reconnect snapshots carry the fresh count)
	out := h.handle(Event{Kind: Crowd, RoomID: "r0", Watchers: 3})
	if len(out) != 1 || out[0].Crowd == nil ||
		out[0].Crowd.RoomID != "r0" || out[0].Crowd.Watchers != 3 {
		t.Fatalf("featured crowd change should emit a Crowd delta, got %#v", out)
	}
	if g := h.games["r0"]; g == nil || g.Watchers != 3 {
		t.Fatalf("registry should hold the new count, got %#v", h.games["r0"])
	}

	// an unchanged count is coalesced away
	out = h.handle(Event{Kind: Crowd, RoomID: "r0", Watchers: 3})
	if len(out) != 0 {
		t.Fatalf("unchanged count should emit no delta, got %#v", out)
	}

	// a pooled room's count updates silently
	out = h.handle(Event{Kind: Crowd, RoomID: "rPool", Watchers: 2})
	if len(out) != 0 {
		t.Fatalf("pooled crowd change should emit no delta, got %#v", out)
	}
	if g := h.games["rPool"]; g == nil || g.Watchers != 2 {
		t.Fatalf("pooled room count should still update, got %#v", h.games["rPool"])
	}

	// a crowd event for an unknown room (e.g. still waiting for players) drops
	out = h.handle(Event{Kind: Crowd, RoomID: "rUnknown", Watchers: 5})
	if len(out) != 0 {
		t.Fatalf("unknown-room crowd change should emit no delta, got %#v", out)
	}
	if _, ok := h.games["rUnknown"]; ok {
		t.Fatalf("crowd event must not create a registry entry")
	}
}

func TestSnapshotReflectsFeaturedSet(t *testing.T) {
	h := newTestHub()
	h.handle(start("a", "g1"))
	h.handle(start("b", "g2"))

	snap := h.snapshot()
	if len(snap.Snapshot) != 2 {
		t.Fatalf("snapshot should hold both featured games, got %d", len(snap.Snapshot))
	}
	if snap.Snapshot[0].RoomID != "a" || snap.Snapshot[1].RoomID != "b" {
		t.Fatalf("snapshot should preserve slot order, got %#v", snap.Snapshot)
	}
}
