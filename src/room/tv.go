package room

import (
	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/tv"
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

	// botColor is the side the engine plays ("w"/"b"), or "" for human-vs-human,
	// so the TV grid can mark exactly which clock is the bot's
	botColor := ""
	if bc := r.players.GetBotColor(); bc != octad.NoColor {
		botColor = bc.String()
	}

	return tv.Event{
		Kind:     kind,
		RoomID:   r.ID,
		GameID:   r.game.ID,
		Variant:  r.game.Variant.Name,
		VsBot:    r.players.HasBot(),
		BotColor: botColor,
		// anchor the board's bottom to a stable player so each side keeps its
		// seat (and score) as colors flip between games; the board flips instead
		Orient:   r.players.AnchorColor().String(),
		OFEN:     r.game.OFEN(),
		LastMove: lastMove,
		Control:  r.game.Variant.Control.Time.Centi(),
		White:    clockState.WhiteTime.Centi(),
		Black:    clockState.BlackTime.Centi(),
		Score:    r.players.ScoreMap(),
		// the clock is paused until the first move starts it; until then the TV
		// grid should show full, static clocks rather than ticking them down
		Running: !clockState.IsPaused,
	}
}
