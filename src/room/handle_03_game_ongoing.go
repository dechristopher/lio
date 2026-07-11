package room

import (
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/lag"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// handleGameOngoing handles moves, player controls, and flag detection
func (r *Instance) handleGameOngoing() {
	connectionListener := channel.Map.GetSockMap(r.ID).Listen()
	defer channel.Map.GetSockMap(r.ID).UnListen(connectionListener)

	// abandon timer; armed only while a player is disconnected. It is created
	// then immediately stopped, so it starts disarmed and is re-armed/stopped
	// via the helpers below. We reuse the single timer (draining a pending fire
	// before each reset) instead of allocating new timers, which avoids the
	// reset-without-drain hazard that could trigger spurious abandonment.
	// A casual bot game gets the short disconnect timeout: its infinite clock
	// and disabled idle timers mean this timer is the only cleanup it has.
	// Human games (casual included) keep the standard reconnect tolerance.
	disconnectTimeout := r.casualDisconnectTimeout()
	abandonTimer := time.NewTimer(disconnectTimeout)
	if !abandonTimer.Stop() {
		<-abandonTimer.C
	}
	defer abandonTimer.Stop()

	armAbandon := func() {
		if !abandonTimer.Stop() {
			select {
			case <-abandonTimer.C:
			default:
			}
		}
		abandonTimer.Reset(disconnectTimeout)
	}
	stopAbandon := func() {
		if !abandonTimer.Stop() {
			select {
			case <-abandonTimer.C:
			default:
			}
		}
	}

	// idle-abandon timer (bot games only). The disconnect abandon timer above
	// keys off socket presence, so it can't catch a human who is still connected
	// but never moves (an idle/backgrounded tab, or someone off watching the
	// home-page TV): the bot would otherwise play the game out to a flag for
	// nobody. This timer fires when
	// humanIdleEligible holds — a bot game, the human has not moved, and it is
	// their turn — and is disarmed the instant they move. Created disarmed;
	// refreshIdle (re)evaluates the condition on entry and after every move.
	idleTimer := time.NewTimer(idleTimeout)
	if !idleTimer.Stop() {
		<-idleTimer.C
	}
	defer idleTimer.Stop()

	refreshIdle := func() {
		// drain any pending fire before re-deciding (the reset-without-drain hazard)
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		if r.humanIdleEligible() {
			idleTimer.Reset(idleTimeout)
		}
	}
	// arm on entry: a rematch in which the human plays Black reaches here with
	// the bot's opening move already played and the no-show human on the clock
	refreshIdle()

	// arm the disconnect timer on entry if a player is already gone: a drop
	// during the state transition lands before this handler's listener exists,
	// so no connection event would ever arm it. A timed game would still end
	// on the clock, but a casual game's infinite clock never flags — the
	// room would outlive its players forever.
	if !r.bothPlayersConnected() {
		armAbandon()
	}

	for {
		select {
		// handle move events
		case move := <-r.moveChannel:
			moveStart := time.Now()
			// if not player's turn, send previous position and continue
			if !r.isTurn(move) {
				channel.Unicast(r.CurrentGameStateMessage(false, false), move.Ctx)
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] got move %+v", r.ID, move)

			// make move and continue routine if move failed
			if ok := r.makeMove(move); !ok {
				continue
			}

			// check to see if the game is over
			isOver, event := r.tryGameOver(move.Ctx, false)
			if isOver {
				// make state transition and exit the gameOngoing routine
				if err := r.event(*event); err != nil {
					panic(err)
				}

				stopAbandon()
				return
			}

			// track move lag for later compensation
			go lag.Move.Track(moveStart)
			util.DebugFlag("lag", str.CRoom, "move lag avg: %s", lag.Move.Get())

			// re-evaluate idle state: a human move disarms the idle timer for
			// good, while the bot's move (re)arms it as we wait on the human
			refreshIdle()

		// handle in-game player controls (resign / draw offer). A control that
		// ends the game (a resignation, or a draw both sides agreed) transitions
		// out of the ongoing state exactly like a move that ends the game.
		case control := <-r.controlChannel:
			isOver, event := r.handleGameControl(control)
			if isOver {
				if err := r.event(*event); err != nil {
					panic(err)
				}
				stopAbandon()
				return
			}

		// handle the engine's verdict on a draw offer in a bot game: an accepted
		// draw ends the game, a declined one is surfaced to the player.
		case eval := <-r.drawEvalChannel:
			isOver, event := r.handleDrawEval(eval)
			if isOver {
				if err := r.event(*event); err != nil {
					panic(err)
				}
				stopAbandon()
				return
			}

		// handle clock events
		case flaggedState := <-r.game.Clock.StateChannel:
			// automatically resign the flagged player. The game mutation runs
			// under stateMu so it can't race readers (CurrentGameStateMessage).
			r.stateMu.Lock()
			if flaggedState.Victor == clock.White {
				r.game.Resign(octad.Black)
			} else {
				r.game.Resign(octad.White)
			}
			r.stateMu.Unlock()

			// run game over routine and get transition event type
			isOver, event := r.tryGameOver(channel.SocketContext{Channel: r.ID, MT: 1}, false)

			// a clock flag followed by a resign must always end the game; if
			// it somehow didn't, abandon the room rather than nil-deref on
			// event or spin on the now-buffered clock state channel
			if !isOver || event == nil {
				util.Error(str.CRoom, "[%s] clock flagged but game not over (victor=%d), abandoning", r.ID, flaggedState.Victor)
				r.abandoned = true
				if err := r.event(EventPlayerAbandons); err != nil {
					panic(err)
				}
				stopAbandon()
				return
			}

			// make state transition and exit the gameOngoing routine
			if err := r.event(*event); err != nil {
				panic(err)
			}

			stopAbandon()
			return
		// handle start/stop of abandon timer when players connect and disconnect
		case <-connectionListener:
			// both players connected? a bot counts as always-connected
			if r.bothPlayersConnected() {
				util.DebugFlag("room", str.CRoom, "[%s] both players connected, cancelling abandon timer", r.ID)
				stopAbandon()
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] players not connected, starting abandon timer", r.ID)
			// start abandon timer if both players are not connected
			armAbandon()
		// figure out who abandoned and resign the game
		case <-abandonTimer.C:
			// determine who is (still) connected
			connected := make(map[octad.Color]bool)

			util.DoBothColors(func(color octad.Color) {
				id, isBot := r.playerInfo(color)
				if isBot {
					// a bot has no socket; it is always considered connected.
					// The early return is required — without it the channel
					// lookup below would overwrite this with false (the
					// original dead-code bug).
					connected[color] = true
					return
				}

				// set whether the player by color is connected
				connected[color] = channel.Map.GetSockMap(r.ID).Connected(id)
			})

			// both players reconnected before the timer fired logic: don't end
			// the game out from under two present players
			if connected[octad.White] && connected[octad.Black] {
				util.DebugFlag("room", str.CRoom, "[%s] both players reconnected, abandon cancelled", r.ID)
				continue
			}

			// decide and apply the outcome under the lock (mutates the game)
			r.stateMu.Lock()
			if !connected[octad.White] && !connected[octad.Black] {
				// neither player is connected: draw the abandoned game
				util.DebugFlag("room", str.CRoom, "[%s] both players abandoned, game drawn", r.ID)
				if err := r.game.Draw(octad.DrawOffer); err != nil {
					r.stateMu.Unlock()
					panic(err)
				}
			} else if !connected[octad.White] {
				util.DebugFlag("room", str.CRoom, "[%s] white abandoned, black wins", r.ID)
				r.game.Resign(octad.White)
			} else {
				util.DebugFlag("room", str.CRoom, "[%s] black abandoned, white wins", r.ID)
				r.game.Resign(octad.Black)
			}
			r.stateMu.Unlock()

			// run game over routine
			r.tryGameOver(channel.SocketContext{Channel: r.ID, MT: 1}, true)

			r.abandoned = true

			// make state transition and exit the gameOngoing routine
			if err := r.event(EventPlayerAbandons); err != nil {
				panic(err)
			}

			return
		// a socket-connected human who never moved this bot game has timed out
		case <-idleTimer.C:
			// a move may have landed just as this fired; re-check so we never
			// abandon a game the human did engage with after all
			if !r.humanIdleEligible() {
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] human idle (no move this game), abandoning bot game", r.ID)

			// resign the idle human so the game has a terminal outcome, then run
			// the same abandon path the disconnect timer uses
			r.stateMu.Lock()
			human := r.players.GetBotColor().Other()
			r.game.Resign(human)
			r.stateMu.Unlock()

			r.tryGameOver(channel.SocketContext{Channel: r.ID, MT: 1}, true)

			r.abandoned = true

			if err := r.event(EventPlayerAbandons); err != nil {
				panic(err)
			}

			return
		}
	}
}
