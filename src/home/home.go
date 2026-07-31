// Package home powers the home page's two live surfaces over one global,
// read-only WebSocket channel (/socket/home): the "live games" grid, and the
// site activity digest (arch/HOME_ACTIVITY_STREAMING.md).
//
// The channel is named for the page rather than for either surface, because it
// carries both. The grid was here first, under the name "tv", and its wire
// types (proto.TVGame and friends) keep that name — they describe the featured-
// game grid specifically, which is still exactly what they are.
//
// A single hub goroutine owns all state.
//
// The **grid** is event-sourced from the rooms: rooms call Publish on game
// start, every move, game over, and room close, and the hub maintains the set
// of live games plus an ordered, fixed-size set of "featured" slots shown in the
// grid. The slot key is the room id (not the game id), so a rematch keeps its
// slot and just streams a new game into it, while a finished game that does not
// rematch ends with the room's cleanup → RoomClosed → its slot is freed and
// backfilled from another live game. That is exactly the "swap out ended matches
// that don't agree to rematch" behaviour, for free.
//
// The **digest** (stat tiles, open challenges, the players roster) is not event
// sourced. It is re-derived in full on a coalesced tick and broadcast only when
// it changes — see digest.go for why the weaker pattern is the right one here.
//
// The hub never imports room (room imports this package); it learns the grid
// from the event stream, and the digest from a source closure injected at wiring
// time. Fan-out reuses the hardened channel layer: a viewer gets a one-shot
// snapshot of the current grid and digest on connect, then a stream of deltas.
package home

import (
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

const (
	// Channel is the global channel key (and /socket/<chan> path segment) the
	// home page's stream is broadcast on. It is not a room, so the WS handler
	// special-cases it (see www/ws/ws.go and IsHome).
	//
	// The name must never collide with a generated room id, which is the same
	// constraint the notification channel has.
	Channel = "home"
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

// EventKind enumerates the room lifecycle transitions the grid cares about.
type EventKind int

const (
	// Start: a game became live (first game of a room, or a rematch).
	Start EventKind = iota
	// Deploy: the room is in its blind deploy phase — the pre-game state where
	// both sides secretly arrange their home rank. It is published on entering
	// the phase, on every lock-in, and on the phase's own re-announce tick, so
	// the grid can show the room filling up before a move is ever played.
	Deploy
	// Move: a featured/live game advanced by a move.
	Move
	// End: a game reached a terminal outcome (the position freezes).
	End
	// Crowd: a room's spectator count changed (someone opened or left the game
	// page between moves). Only Kind/RoomID/Watchers are meaningful.
	Crowd
	// RoomClosed: the room was torn down (no rematch / abandon / cancel); its
	// slot is freed and backfilled.
	RoomClosed

	// dirtyOnly carries no grid state at all: it only tells the hub that the
	// digest may have changed (see MarkDirty). It is unexported because it is
	// not a room lifecycle transition — callers reach it through MarkDirty,
	// which says what it is for.
	dirtyOnly
)

// Event is the room → hub message. All fields except Kind/RoomID are ignored for
// RoomClosed. Clocks are centi-seconds (matching proto.ClockPayload).
type Event struct {
	Kind     EventKind
	RoomID   string
	GameID   string
	Variant  string
	Deploy   bool
	Watchers int
	VsBot    bool
	Orient   string
	OFEN     string
	LastMove string
	Control  int64
	White    int64
	Black    int64
	// WhiteSeat / BlackSeat are the two seats' display identities (name, title
	// badge, bot persona). They are resolved once by the room, where the seats
	// live, rather than re-derived per viewer: the TV stream has no viewer to
	// address, so every card shows the same names.
	WhiteSeat proto.TVSeat
	BlackSeat proto.TVSeat
	// Casual marks an untimed game: the grid renders its clocks as a static ∞
	// instead of ticking down the (effectively infinite) real values.
	Casual bool
	Score  proto.ScorePayload
	// RaceTo is the room's match target in points (0 for a single game). The
	// grid captions a match with it so a viewer reads the score chips as a
	// running match score rather than a one-off game result.
	RaceTo int
	// Winner / Reason describe a finished game's outcome ("w"/"b"/"d" plus the
	// method code). The room fills them on End only; every other kind leaves
	// them empty. See proto.TVGame.
	Winner string
	Reason string
	// Running reports whether a side's clock is actually being charged, so the
	// client can hold the times static rather than ticking them down through
	// every state where the server is not draining anyone (see proto.TVGame).
	Running bool
	// Deploying / PhaseLeft / PhaseTotal describe the pre-game phase the room is
	// in: the blind deploy, then the first-move grace. See proto.TVGame.
	Deploying  bool
	PhaseLeft  int64
	PhaseTotal int64
}

// hubMsg multiplexes the inbound request kinds onto the hub's single inbound
// channel: a room lifecycle event, a new viewer asking for a snapshot, or the
// one-shot injection of the digest source.
type hubMsg struct {
	ev      *Event
	sock    *channel.Socket
	sources *sources
}

// hub owns the live-game registry, the featured slots, and the activity digest.
// All fields are touched only by run (a single goroutine), so they need no
// synchronization.
type hub struct {
	in       chan hubMsg
	games    map[string]*proto.TVGame // every live room, keyed by room id
	featured []string                 // ordered featured room ids, len <= Cap
	digest   digestState              // the activity region; see digest.go
}

var theHub = &hub{
	in:       make(chan hubMsg, inBuffer),
	games:    make(map[string]*proto.TVGame),
	featured: make([]string, 0, Cap),
}

// Up starts the hub goroutine and pre-creates the home channel's SockMap so it
// is ready to broadcast before the first viewer connects. Wired into
// systems.Run.
//
// The digest half of the hub stays dormant until SetSource is called: the
// source needs room and presence, which this package must not import, so it is
// injected from www once those are wired (see digest.go).
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

// Connect asks the hub to send the current grid snapshot and activity digest to
// a freshly connected viewer's socket. It is called from the WS connection
// goroutine after the socket is tracked, and routes through the hub so both
// snapshots are built from the authoritative single-owner state.
func Connect(s *channel.Socket) {
	theHub.in <- hubMsg{sock: s}
}

// IsHome reports whether the given channel id is the global home channel (as
// opposed to a room id). Used by the WS handler to skip the room-existence
// check.
func IsHome(id string) bool {
	return id == Channel
}

// run is the hub's single owning goroutine. It owns both surfaces: the grid,
// which is event-sourced and answers each event immediately, and the digest,
// which is re-derived on the ticker below (see digest.go).
//
// One goroutine for both is deliberate. They share the connect path — a viewer
// gets one snapshot covering both — and keeping them in the same owner means
// neither needs a lock to read what the other holds.
func (h *hub) run() {
	tick := time.NewTicker(digestFloor)
	defer tick.Stop()

	for {
		select {
		case m := <-h.in:
			switch {
			case m.sources != nil:
				h.digest.src = m.sources.digest
			case m.sock != nil:
				snap := h.snapshot()
				m.sock.Enqueue(snap.Marshal())
				h.connectDigest(m.sock)
			case m.ev != nil:
				// every room event moves the digest as well as the grid: a game
				// starting changes the live count, a room closing may free an
				// open challenge, and either can change who reads as seated
				h.digest.dirty = true
				for _, p := range h.handle(*m.ev) {
					h.broadcast(p)
				}
			}
		case <-tick.C:
			h.tick()
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

	// Deploy rides the Move path deliberately: its "claim a free slot if this
	// room is new to me, otherwise patch the slot in place" behaviour is exactly
	// what a deploy-phase update needs, and it means a room shows up on the grid
	// the moment it starts deploying rather than only once it has a position.
	case Move, End, Deploy:
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

	case Crowd:
		// count-only patch for a room we already track; a Crowd event for an
		// unknown room (e.g. still waiting for players) is dropped — the Start
		// event carries a fresh count when the game goes live
		g, known := h.games[ev.RoomID]
		if !known || g.Watchers == ev.Watchers {
			return nil
		}
		g.Watchers = ev.Watchers
		if h.featuredIndex(ev.RoomID) >= 0 {
			return []proto.TVPayload{{Crowd: &proto.TVCrowd{
				RoomID:   ev.RoomID,
				Watchers: ev.Watchers,
			}}}
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
		RoomID:     ev.RoomID,
		GameID:     ev.GameID,
		Variant:    ev.Variant,
		Deploy:     ev.Deploy,
		Watchers:   ev.Watchers,
		VsBot:      ev.VsBot,
		Orient:     ev.Orient,
		OFEN:       ev.OFEN,
		LastMove:   ev.LastMove,
		Control:    ev.Control,
		White:      ev.White,
		Black:      ev.Black,
		WhiteSeat:  ev.WhiteSeat,
		BlackSeat:  ev.BlackSeat,
		Casual:     ev.Casual,
		Score:      ev.Score,
		RaceTo:     ev.RaceTo,
		Winner:     ev.Winner,
		Reason:     ev.Reason,
		Running:    ev.Running && !over,
		Over:       over,
		Deploying:  ev.Deploying,
		PhaseLeft:  ev.PhaseLeft,
		PhaseTotal: ev.PhaseTotal,
	}
}

// copyGame returns a heap copy of g so a delta payload never aliases the hub's
// registry entry (deltas outlive the handle call only until run marshals them,
// but copying keeps that invariant local and obvious).
func copyGame(g proto.TVGame) *proto.TVGame {
	cp := g
	return &cp
}
