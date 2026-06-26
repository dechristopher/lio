// Package tv powers the home-page "live games" widget: a single global,
// read-only WebSocket channel (/socket/tv) that streams a capped grid of
// in-progress games to every viewer on the home page.
//
// A single hub goroutine owns all state and is event-sourced from the rooms:
// rooms call Publish on game start, every move, game over, and room close, and
// the hub maintains the set of live games plus an ordered, fixed-size set of
// "featured" slots shown in the grid. The slot key is the room id (not the game
// id), so a rematch keeps its slot and just streams a new game into it, while a
// finished game that does not rematch ends with the room's cleanup → RoomClosed
// → its slot is freed and backfilled from another live game. That is exactly the
// "swap out ended matches that don't agree to rematch" behaviour, for free.
//
// The hub never imports room (room imports tv); it learns everything it needs
// from the event stream. Fan-out reuses the hardened channel layer: a viewer
// gets a one-shot snapshot of the current grid on connect, then a stream of
// add/move/remove deltas for featured rooms only.
package tv

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

const (
	// Channel is the global channel key (and /socket/<chan> path segment) the
	// TV stream is broadcast on. It is not a room, so the WS handler special-
	// cases it (see www/ws/ws.go and IsTV).
	Channel = "tv"
	// Cap is the maximum number of games shown in the grid at once. Additional
	// live games wait in the pool and are promoted as featured slots free up.
	Cap = 6
	// inBuffer bounds the hub's inbound queue. Publish is non-blocking and drops
	// on a full buffer (a dropped move just leaves a board briefly stale until
	// the next move or a reconnect snapshot), matching the channel layer's
	// drop-slow-consumer philosophy. Sized generously so this effectively never
	// happens in practice.
	inBuffer = 256
)

// EventKind enumerates the room lifecycle transitions the TV hub cares about.
type EventKind int

const (
	// Start: a game became live (first game of a room, or a rematch).
	Start EventKind = iota
	// Move: a featured/live game advanced by a move.
	Move
	// End: a game reached a terminal outcome (the position freezes).
	End
	// RoomClosed: the room was torn down (no rematch / abandon / cancel); its
	// slot is freed and backfilled.
	RoomClosed
)

// Event is the room → hub message. All fields except Kind/RoomID are ignored for
// RoomClosed. Clocks are centi-seconds (matching proto.ClockPayload).
type Event struct {
	Kind     EventKind
	RoomID   string
	GameID   string
	Variant  string
	VsBot    bool
	OFEN     string
	LastMove string
	Control  int64
	White    int64
	Black    int64
	Score    proto.ScorePayload
	// Running reports whether the game clock is live. It is false before the
	// first move (the clock is paused until White moves), so the client can hold
	// the clocks static instead of ticking them down on an unstarted game.
	Running bool
}

// hubMsg multiplexes the two inbound request kinds onto the hub's single inbound
// channel: a room lifecycle event, or a new viewer asking for a snapshot.
type hubMsg struct {
	ev   *Event
	sock *channel.Socket
}

// hub owns the live-game registry and featured slots. All fields are touched
// only by run (a single goroutine), so they need no synchronization.
type hub struct {
	in       chan hubMsg
	games    map[string]*proto.TVGame // every live room, keyed by room id
	featured []string                 // ordered featured room ids, len <= Cap
}

var theHub = &hub{
	in:       make(chan hubMsg, inBuffer),
	games:    make(map[string]*proto.TVGame),
	featured: make([]string, 0, Cap),
}

// Up starts the hub goroutine and pre-creates the TV channel's SockMap so it is
// ready to broadcast before the first viewer connects. Wired into systems.Run.
func Up() {
	channel.Map.GetSockMap(Channel)
	go theHub.run()
}

// Publish hands a room lifecycle event to the hub. It never blocks the caller
// (the room routine): if the hub's inbound queue is full the event is dropped.
func Publish(e Event) {
	select {
	case theHub.in <- hubMsg{ev: &e}:
	default:
		// hub saturated; drop. The next event / a reconnect snapshot reconciles.
	}
}

// Connect asks the hub to send the current grid snapshot to a freshly connected
// viewer's socket. It is called from the WS connection goroutine after the
// socket is tracked, and routes through the hub so the snapshot is built from
// the authoritative single-owner state.
func Connect(s *channel.Socket) {
	theHub.in <- hubMsg{sock: s}
}

// IsTV reports whether the given channel id is the global TV channel (as opposed
// to a room id). Used by the WS handler to skip the room-existence check.
func IsTV(id string) bool {
	return id == Channel
}

// run is the hub's single owning goroutine.
func (h *hub) run() {
	for m := range h.in {
		if m.sock != nil {
			snap := h.snapshot()
			m.sock.Enqueue(snap.Marshal())
			continue
		}
		for _, p := range h.handle(*m.ev) {
			h.broadcast(p)
		}
	}
}

// broadcast marshals a delta and fans it out to every TV viewer via the channel
// layer. Marshalling happens synchronously here, so the returned payloads may
// safely alias hub state.
func (h *hub) broadcast(p proto.TVPayload) {
	channel.Broadcast(p.Marshal(), channel.SocketContext{Channel: Channel})
}

// handle applies a room event to the registry and returns the deltas to
// broadcast (empty for non-featured churn). It is pure with respect to the
// network — all fan-out happens in run/broadcast — which keeps it unit-testable
// without any sockets.
func (h *hub) handle(ev Event) []proto.TVPayload {
	switch ev.Kind {
	case Start:
		g := tvGameFrom(ev, false)
		h.games[ev.RoomID] = &g
		if h.featuredIndex(ev.RoomID) >= 0 {
			// rematch / restart within an existing slot: stream the new game in.
			// The client treats Add for a known room as a replace (new GameID).
			return []proto.TVPayload{{Add: copyGame(g)}}
		}
		if len(h.featured) < Cap {
			h.featured = append(h.featured, ev.RoomID)
			return []proto.TVPayload{{Add: copyGame(g)}}
		}
		return nil

	case Move, End:
		g := tvGameFrom(ev, ev.Kind == End)
		_, known := h.games[ev.RoomID]
		h.games[ev.RoomID] = &g
		// a game we never saw start (hub came up mid-game) is adopted as if it
		// had just started, so it can still claim a free slot
		if !known && h.featuredIndex(ev.RoomID) < 0 {
			if len(h.featured) < Cap {
				h.featured = append(h.featured, ev.RoomID)
				return []proto.TVPayload{{Add: copyGame(g)}}
			}
			return nil
		}
		if h.featuredIndex(ev.RoomID) >= 0 {
			return []proto.TVPayload{{Move: copyGame(g)}}
		}
		return nil

	case RoomClosed:
		delete(h.games, ev.RoomID)
		i := h.featuredIndex(ev.RoomID)
		if i < 0 {
			return nil
		}
		h.featured = append(h.featured[:i], h.featured[i+1:]...)
		out := []proto.TVPayload{{Remove: ev.RoomID}}
		// backfill the freed slot from any live room not already featured
		if rid := h.firstUnfeatured(); rid != "" {
			h.featured = append(h.featured, rid)
			out = append(out, proto.TVPayload{Add: copyGame(*h.games[rid])})
		}
		return out
	}
	return nil
}

// snapshot builds the full current grid for a newly connected viewer.
func (h *hub) snapshot() proto.TVPayload {
	games := make([]proto.TVGame, 0, len(h.featured))
	for _, rid := range h.featured {
		if g, ok := h.games[rid]; ok {
			games = append(games, *g)
		}
	}
	return proto.TVPayload{Snapshot: games}
}

// featuredIndex returns the slot index of a room id, or -1 if not featured.
func (h *hub) featuredIndex(roomID string) int {
	for i, rid := range h.featured {
		if rid == roomID {
			return i
		}
	}
	return -1
}

// firstUnfeatured returns the room id of any live game not currently featured,
// or "" if every live game already holds a slot. Map iteration order is
// unspecified, which is fine: backfill order among waiting games is arbitrary.
func (h *hub) firstUnfeatured() string {
	for rid := range h.games {
		if h.featuredIndex(rid) < 0 {
			return rid
		}
	}
	return ""
}

// tvGameFrom projects a room Event onto the wire display struct.
func tvGameFrom(ev Event, over bool) proto.TVGame {
	return proto.TVGame{
		RoomID:   ev.RoomID,
		GameID:   ev.GameID,
		Variant:  ev.Variant,
		VsBot:    ev.VsBot,
		OFEN:     ev.OFEN,
		LastMove: ev.LastMove,
		Control:  ev.Control,
		White:    ev.White,
		Black:    ev.Black,
		Score:    ev.Score,
		Running:  ev.Running && !over,
		Over:     over,
	}
}

// copyGame returns a heap copy of g so a delta payload never aliases the hub's
// registry entry (deltas outlive the handle call only until run marshals them,
// but copying keeps that invariant local and obvious).
func copyGame(g proto.TVGame) *proto.TVGame {
	cp := g
	return &cp
}
