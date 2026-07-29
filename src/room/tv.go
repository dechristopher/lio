package room

import (
	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/tv"
	"github.com/dechristopher/lio/www/ws/proto"
)

// tvEvent builds a tv.Event of the given kind, locking stateMu itself. Used at
// call sites that do not already hold the lock (e.g. the game-start broadcast).
func (r *Instance) tvEvent(kind tv.EventKind) tv.Event {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.tvEventLocked(kind)
}

// tvEventLocked builds the home-page TV stream event describing the current
// game. The caller must hold stateMu (it reads the game, clock, and players).
// Clocks are reported in centi-seconds, matching proto.ClockPayload, and reflect
// the post-flip state after a move so the grid's clock bars track the live game.
func (r *Instance) tvEventLocked(kind tv.EventKind) tv.Event {
	clockState := r.game.Clock.State(true)

	lastMove := ""
	if moves := r.game.Moves(); len(moves) > 0 {
		lastMove = moves[len(moves)-1].String()
	}

	return tv.Event{
		Kind:     kind,
		RoomID:   r.ID,
		GameID:   r.game.ID,
		Variant:  r.game.Variant.Name,
		Deploy:   r.game.Variant.Deploy,
		Watchers: r.watchersLocked(),
		VsBot:    r.players.HasBot(),
		// anchor the board's bottom to a stable player so each side keeps its
		// seat (and score) as colors flip between games; the board flips instead
		Orient:    r.players.AnchorColor().String(),
		OFEN:      r.game.OFEN(),
		LastMove:  lastMove,
		Control:   r.game.Variant.Control.Time.Centi(),
		White:     clockState.WhiteTime.Centi(),
		Black:     clockState.BlackTime.Centi(),
		WhiteSeat: r.tvSeatLocked(octad.White),
		BlackSeat: r.tvSeatLocked(octad.Black),
		// untimed casual game: the grid shows static ∞ clocks
		Casual: r.game.Variant.Casual,
		Score:  r.players.ScoreMap(),
		// the clock is paused until the first move starts it; until then the TV
		// grid should show full, static clocks rather than ticking them down
		Running: !clockState.IsPaused,
	}
}

// tvSeatLocked resolves one seat's identity for the TV grid: the difficulty
// persona's name and piece glyph for the engine, the account username plus its
// title badge for a logged-in human, and "Anonymous" for an anonymous one. The
// caller must hold stateMu (it reads the seats).
//
// Like seatArchiveName — and unlike the room view's DisplayName, which returns
// "" so the page can pick "You"/"Anonymous" per viewer — the TV stream has no
// viewer to address: one card is broadcast to everyone, so an anonymous seat is
// spelled out here rather than left for a client that could never resolve it.
func (r *Instance) tvSeatLocked(color octad.Color) proto.TVSeat {
	p := r.players[color]
	if p == nil {
		return proto.TVSeat{Name: "Anonymous"}
	}
	if p.IsBot {
		persona := r.botPersona()
		return proto.TVSeat{Name: persona.Name, Bot: true, Glyph: persona.Glyph}
	}
	if p.Username == "" {
		return proto.TVSeat{Name: "Anonymous"}
	}
	return proto.TVSeat{
		Name:      p.Username,
		Title:     p.Title.Code,
		TitleName: p.Title.Name,
	}
}

// watchersLocked counts the spectators connected to this room: distinct uids
// on the room channel minus the connected seats (the same derivation as the
// crowd broadcast, see handlers.HandleCrowd). The caller must hold stateMu
// (it reads the seats). Peek never creates a SockMap, and a nil SockMap
// counts as zero.
func (r *Instance) watchersLocked() int {
	sockMap := channel.Map.Peek(r.ID)
	if sockMap == nil {
		return 0
	}
	white, black := r.playerIDsLocked()
	watchers := sockMap.Length()
	if sockMap.Connected(white) {
		watchers--
	}
	if sockMap.Connected(black) {
		watchers--
	}
	return watchers
}
