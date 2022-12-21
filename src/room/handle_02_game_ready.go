package room

import (
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// handle waiting for white to make first move and start game
func (r *Instance) handleGameReady() {
	cleanupTimer := time.NewTimer(gameReadyExpiryTime)
	defer cleanupTimer.Stop()

	// broadcast reset board state to all
	channel.Broadcast(r.GetSerializedGameState(), channel.SocketContext{
		Channel: r.ID,
		MT:      2,
	})

	util.DebugFlag("room", str.CRoom, "[%s] waiting for white to move", r.ID)

	// request engine move immediately
	if r.players.GetBotColor() == octad.White {
		util.DebugFlag("room", str.CRoom, "[%s] engine making first move..", r.ID)
		r.requestEngineMove()
	}

	for {
		select {
		case move := <-r.moveChannel:
			util.DebugFlag("room", str.CRoom, "[%s] got move %s from %s (%s / %s)", r.ID, move.Move.Uoi, move.Player, r.game.White, r.game.Black)

			// don't allow moves out of order
			if !r.isTurn(move) {
				channel.Unicast(r.GetSerializedGameState(), move.Ctx)
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] white (%s) trying to make first move", r.ID, move.Player)
			// start game clock on first move
			if r.game.Clock.State(true).IsPaused {
				util.DebugFlag("room", str.CRoom, "[%s] starting clock", r.ID)
				r.game.Clock.Start()
			}

			// make move and continue routine if move failed
			if ok := r.makeMove(move); !ok {
				util.DebugFlag("room", str.CRoom, "[%s] invalid first move, resetting clock", r.ID)
				r.game.Clock.Reset()

				// re-request engine first move
				if r.players.GetBotColor() == octad.White {
					r.requestEngineMove()
				}
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] white made first move, game (%s) starting", r.ID, r.game.ID)

			// transition game state to GameOngoing
			err := r.event(EventStartGame)
			if err != nil {
				panic(err)
			}

			return
		case <-cleanupTimer.C:
			r.abandoned = true
			// game expired, white timed out making first move
			util.DebugFlag("room", str.CRoom, "[%s] game expired, white timed out making first move, cleaning up", r.ID)
			err := r.event(EventPlayerAbandons)
			if err != nil {
				panic(err)
			}
			return
		}
	}
}
