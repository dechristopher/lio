package room

import (
	"github.com/dechristopher/lioctad/channel"
	"github.com/dechristopher/lioctad/clock"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/octad"
)

// handleGameOngoing handles moves, player controls, and flag detection
func (r *Instance) handleGameOngoing() {
	// TODO handle player abandons
	// timer for a certain percentage of game time up to a given max
	// maybe 30 seconds?
	// player automatically resigns if they abandon
	//connectionListener := channel.Map[r.ID].Listen()

	for {
		select {
		// handle move events
		case move := <-r.moveChannel:
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
			isOver, event := r.tryGameOver(move.Ctx)
			if isOver {
				// make state transition and exit the gameOngoing routine
				err := r.event(*event)
				if err != nil {
					panic(err)
				}

				return
			}

		// handle clock events
		case flaggedState := <-r.game.Clock.StateChannel:
			//automatically resign game if clock expires
			if flaggedState.Victor == clock.White {
				r.game.Resign(octad.Black)
			} else {
				r.game.Resign(octad.White)
			}

			// run game over routine and get transition event type
			_, event := r.tryGameOver(channel.SocketContext{Channel: r.ID, MT: 1})

			// make state transition and exit the gameOngoing routine
			err := r.event(*event)
			if err != nil {
				panic(err)
			}

			return
		}
	}
}
