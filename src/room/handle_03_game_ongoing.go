package room

import (
	"time"

	"github.com/dechristopher/octad"

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
	abandonTimer := time.NewTimer(abandonTimeout)
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
		abandonTimer.Reset(abandonTimeout)
	}
	stopAbandon := func() {
		if !abandonTimer.Stop() {
			select {
			case <-abandonTimer.C:
			default:
			}
		}
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
			playersConnected := util.BothColors(func(color octad.Color) bool {
				id, isBot := r.playerInfo(color)
				if isBot {
					return true
				}
				// return whether the player is connected
				return channel.Map.GetSockMap(r.ID).Get(id) != nil
			})

			if playersConnected {
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
				connected[color] = channel.Map.GetSockMap(r.ID).Get(id) != nil
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
		}
	}
}
