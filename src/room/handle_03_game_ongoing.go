package room

import (
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lioctad/channel"
	"github.com/dechristopher/lioctad/clock"
	"github.com/dechristopher/lioctad/lag"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// handleGameOngoing handles moves, player controls, and flag detection
func (r *Instance) handleGameOngoing() {
	connectionListener := channel.Map[r.ID].Listen()
	defer channel.Map[r.ID].UnListen(connectionListener)

	// set up abandon timer beyond any regular game duration
	var abandonTimer = time.NewTimer(time.Hour)

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
				err := r.event(*event)
				if err != nil {
					panic(err)
				}

				// stop abandon timer
				abandonTimer.Stop()

				return
			}

			// track move lag for later compensation
			lag.Move.Track(moveStart)
			util.DebugFlag("lag", str.CRoom, "move lag avg: %s", lag.Move.Get())

		// handle clock events
		case flaggedState := <-r.game.Clock.StateChannel:
			//automatically resign game if clock expires
			if flaggedState.Victor == clock.White {
				r.game.Resign(octad.Black)
			} else {
				r.game.Resign(octad.White)
			}

			// run game over routine and get transition event type
			_, event := r.tryGameOver(channel.SocketContext{Channel: r.ID, MT: 1}, false)

			// make state transition and exit the gameOngoing routine
			err := r.event(*event)
			if err != nil {
				panic(err)
			}

			// stop abandon timer
			abandonTimer.Stop()

			return
		// handle start/stop of abandon timer when players connect and disconnect
		case <-connectionListener:
			// both players connected, no issues
			playersConnected := util.BothColors(func(color octad.Color) bool {
				if r.players[color].IsBot {
					return true
				}

				// return whether the player is connected
				return channel.Map[r.ID].Get(r.players[color].ID) != nil
			})

			if playersConnected {
				util.DebugFlag("room", str.CRoom, "[%s] both players connected, cancelling abandon timer", r.ID)
				// stop abandonTimer
				if abandonTimer != nil {
					abandonTimer.Stop()
				}
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] players not connected, starting abandon timer", r.ID)

			// start abandon timer if both players are not connected
			if abandonTimer == nil {
				abandonTimer = time.NewTimer(abandonTimeout)
			} else {
				abandonTimer.Reset(abandonTimeout)
			}
		// figure out who abandoned and resign the game
		case <-abandonTimer.C:
			// determine who isn't connected
			connected := make(map[octad.Color]bool)

			util.DoBothColors(func(color octad.Color) {
				if r.players[color].IsBot {
					connected[color] = true
				}

				// set whether the player by color is connected
				connected[color] = channel.Map[r.ID].Get(r.players[color].ID) != nil
			})

			// if both players abandoned
			if util.BothColors(func(color octad.Color) bool {
				return connected[color]
			}) {
				util.DebugFlag("room", str.CRoom, "[%s] both players abandoned, game drawn", r.ID)
				// draw the game immediately
				err := r.game.Draw(octad.DrawOffer)
				if err != nil {
					panic(err)
				}
			} else {
				// otherwise find abandoning player and resign them
				if !connected[octad.White] {
					util.DebugFlag("room", str.CRoom, "[%s] white abandoned, black wins", r.ID)
					r.game.Resign(octad.White)
				} else {
					util.DebugFlag("room", str.CRoom, "[%s] black abandoned, white wins", r.ID)
					r.game.Resign(octad.Black)
				}
			}

			// run game over routine
			r.tryGameOver(channel.SocketContext{Channel: r.ID, MT: 1}, true)

			r.abandoned = true

			// make state transition and exit the gameOngoing routine
			err := r.event(EventPlayerAbandons)
			if err != nil {
				panic(err)
			}

			return
		}
	}
}
