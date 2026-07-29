package room

import (
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/clock"
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

	// the room is mid blind-deploy exactly while the deadline is set: handleDeploy
	// stamps it on entry and deployAndStart clears it at the reveal
	deploying := !r.deployDeadline.IsZero()

	// the two pre-game timers take the grid's dial in turn, so only one of them
	// is ever live: the deploy phase's auto-fill deadline, then the post-reveal
	// first-move grace the clock owns (State.PreStart)
	var phaseLeft, phaseTotal int64
	if deploying {
		if d := time.Until(r.deployDeadline); d > 0 {
			phaseLeft = clock.ToCTime(d).Centi()
		}
		phaseTotal = clock.ToCTime(deployTimeout).Centi()
	} else {
		phaseLeft = clockState.PreStart.Centi()
		phaseTotal = r.game.Variant.Control.PreStart.Centi()
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
		// only report the clock as running when a side is actually being charged.
		// The clock is unpaused for the whole pre-start grace but drains nobody
		// during it, so reporting it live is what used to make the grid tick
		// White down and then snap back on the first move.
		Running:    !clockState.IsPaused && !deploying && phaseLeft == 0,
		Deploying:  deploying,
		PhaseLeft:  phaseLeft,
		PhaseTotal: phaseTotal,
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
	// committed its blind-deploy arrangement. r.deployed is nil outside the
	// phase, so this reads false for every seat during normal play.
	_, locked := r.deployed[color]

	p := r.players[color]
	if p == nil {
		return proto.TVSeat{Name: "Anonymous", Locked: locked}
	}
	if p.IsBot {
		persona := r.botPersona()
		return proto.TVSeat{Name: persona.Name, Bot: true, Glyph: persona.Glyph, Locked: locked}
	}
	if p.Username == "" {
		return proto.TVSeat{Name: "Anonymous", Locked: locked}
	}
	return proto.TVSeat{
		Name:      p.Username,
		Title:     p.Title.Code,
		TitleName: p.Title.Name,
		Locked:    locked,
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
