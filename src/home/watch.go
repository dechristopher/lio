package home

import (
	"strings"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

// Per-connection room watches, for the username hover card
// (arch/PLAYER_CARD.md).
//
// The hub is already the right owner for this and needs no new plumbing to do
// it: it receives *every* room's lifecycle events, not only the featured ones,
// and h.games is the full live set. The featured slots are a view over that
// state for the home grid; a watch is a second, per-connection view over the
// same state.
//
// # Why a watch is keyed by (channel, uid), not by socket
//
// The inbound handler receives a channel.SocketContext, which names the channel
// and the session but not the individual connection — so a key is the finest
// identity available without widening channel.Handler for this one caller.
//
// The cost is that two tabs of one session on the same channel share a watch:
// the second tab's hover replaces the first's, and closing either card cancels
// both. The failure that produces is a card whose board stops updating while it
// is open, which the static snapshot it opened with already covers. That is a
// fair trade for not threading a connection identity through the whole ws
// handler surface.
//
// # Cleanup
//
// A page navigating away closes its socket without cancelling anything, so
// entries must not depend on the client's goodbye. Delivery resolves the key
// through the channel directory, so a dead key simply finds no sockets — and
// that is the moment it is dropped. The registry therefore self-prunes on the
// next event for the room being watched, and a watch on a room that never moves
// again is released when the room closes.

// watchKey identifies the connections a watch delivers to: every socket a
// session holds on one channel.
type watchKey struct {
	channel string
	uid     string
}

// watchState is the hub's watch registry. Both maps are owned by the hub
// goroutine, like every other field of hub.
type watchState struct {
	// byRoom answers "who is watching this room" on every event.
	byRoom map[string]map[watchKey]struct{}
	// byKey answers "what is this connection already watching", so a new
	// request can leave the room it replaces.
	byKey map[watchKey]string
}

func newWatchState() watchState {
	return watchState{
		byRoom: make(map[string]map[watchKey]struct{}),
		byKey:  make(map[watchKey]string),
	}
}

// watchReq is the inbound "follow this room" message. An empty RoomID cancels
// whatever this connection was watching.
type watchReq struct {
	key    watchKey
	roomID string
}

// gameQuery asks the hub which live game a named player is sitting in. The
// reply channel is buffered so the hub never blocks on a caller that gave up.
type gameQuery struct {
	username string
	reply    chan *proto.TVGame
}

// queryTimeout bounds how long a page render waits on the hub for a player's
// live game. The hub answers in microseconds; this only exists so a request
// can never be pinned behind a saturated inbound queue. Missing the deadline
// costs the card its board, not the card.
const queryTimeout = 250 * time.Millisecond

// Watch registers a connection's standing interest in one room's live game, or
// cancels it when roomID is empty. Called from the WS handler goroutine.
//
// Non-blocking, like Publish: a saturated hub drops the request, and the card
// keeps the static snapshot it opened with.
func Watch(channelName, uid, roomID string) {
	if uid == "" {
		return
	}
	req := &watchReq{
		key:    watchKey{channel: channelName, uid: uid},
		roomID: roomID,
	}
	select {
	case theHub.in <- hubMsg{watch: req}:
	default:
		// hub saturated; drop. The card falls back to its static snapshot.
	}
}

// LiveGameFor returns the live game the named account is currently playing in,
// if any. It is the hover card's initial state: the card paints a full board
// from the page's own response, and the socket stream then keeps it moving.
//
// Answering from the hub rather than from the room registry is deliberate — it
// is the same struct, resolved the same way, that the watch stream will send,
// so the card's first frame and its updates can never disagree about what the
// game looks like.
func LiveGameFor(username string) (proto.TVGame, bool) {
	if username == "" {
		return proto.TVGame{}, false
	}
	q := &gameQuery{username: username, reply: make(chan *proto.TVGame, 1)}
	select {
	case theHub.in <- hubMsg{query: q}:
	default:
		return proto.TVGame{}, false
	}
	select {
	case g := <-q.reply:
		if g == nil {
			return proto.TVGame{}, false
		}
		return *g, true
	case <-time.After(queryTimeout):
		return proto.TVGame{}, false
	}
}

// applyWatch moves one connection's watch to a new room (or clears it) and
// answers immediately with the room's current state, so a card that opened
// without a snapshot still fills in.
func (h *hub) applyWatch(req *watchReq) {
	if prev, ok := h.watch.byKey[req.key]; ok {
		h.dropWatch(req.key, prev)
	}
	if req.roomID == "" {
		return
	}
	set := h.watch.byRoom[req.roomID]
	if set == nil {
		set = make(map[watchKey]struct{}, 1)
		h.watch.byRoom[req.roomID] = set
	}
	set[req.key] = struct{}{}
	h.watch.byKey[req.key] = req.roomID
	h.pushWatchTo(req.key, req.roomID)
}

// dropWatch removes one key's membership of one room, forgetting the room once
// nobody is watching it.
func (h *hub) dropWatch(key watchKey, roomID string) {
	delete(h.watch.byKey, key)
	set := h.watch.byRoom[roomID]
	if set == nil {
		return
	}
	delete(set, key)
	if len(set) == 0 {
		delete(h.watch.byRoom, roomID)
	}
}

// pushWatch sends a room's current state to everyone watching it. Called after
// every event the hub applies, so the frame reflects the registry *after*
// handle has updated (or deleted) the game.
func (h *hub) pushWatch(roomID string) {
	if len(h.watch.byRoom[roomID]) == 0 {
		return
	}
	payload := h.watchPayload(roomID)
	data := payload.Marshal()
	// collected first: delivery prunes dead keys, which mutates the set
	keys := make([]watchKey, 0, len(h.watch.byRoom[roomID]))
	for key := range h.watch.byRoom[roomID] {
		keys = append(keys, key)
	}
	for _, key := range keys {
		if !deliver(key, data) {
			h.dropWatch(key, roomID)
		}
	}
	// a room that has ended holds nobody: the game is gone, so no later event
	// can arrive for it and a watch on it would never be pruned
	if payload.Gone {
		for _, key := range keys {
			h.dropWatch(key, roomID)
		}
	}
}

// pushWatchTo answers one key with a room's current state.
func (h *hub) pushWatchTo(key watchKey, roomID string) {
	payload := h.watchPayload(roomID)
	if !deliver(key, payload.Marshal()) {
		h.dropWatch(key, roomID)
	}
}

// watchPayload builds the wire frame for a room: its live game, or a Gone
// marker when the hub holds none.
func (h *hub) watchPayload(roomID string) proto.WatchPayload {
	g, ok := h.games[roomID]
	if !ok {
		return proto.WatchPayload{RoomID: roomID, Gone: true}
	}
	cp := *g
	return proto.WatchPayload{RoomID: roomID, Game: &cp}
}

// liveGameFor finds the live game a named account is seated in. Seat names are
// the display identities the room resolved (proto.TVSeat), so a bot seat is
// matched by its persona name and must be excluded — "Queen" is a difficulty,
// not an account.
//
// A linear scan over the live rooms, which is what the featured-slot bookkeeping
// beside it already does. The set is bounded by concurrent games on the site,
// and the alternative — a name index maintained across every start, rematch and
// close — is a second copy of this truth for a lookup that costs microseconds.
func (h *hub) liveGameFor(username string) *proto.TVGame {
	for _, g := range h.games {
		if seatIs(g.WhiteSeat, username) || seatIs(g.BlackSeat, username) {
			cp := *g
			return &cp
		}
	}
	return nil
}

// seatIs reports whether a seat is held by the named account. Case-insensitive,
// like every other username comparison on the site.
func seatIs(s proto.TVSeat, username string) bool {
	return !s.Bot && s.Name != "" && strings.EqualFold(s.Name, username)
}

// deliver queues a frame for every connection a key names, and reports whether
// any still exist. A false answer is how the registry learns a page navigated
// away — see the cleanup note at the top of this file.
func deliver(key watchKey, data []byte) bool {
	sockMap := channel.Map.Peek(key.channel)
	if sockMap == nil {
		return false
	}
	socks := sockMap.SocketsFor(key.uid)
	if len(socks) == 0 {
		return false
	}
	for _, sock := range socks {
		sock.Enqueue(data)
	}
	return true
}
