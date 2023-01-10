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
	var cleanupTimer = time.NewTimer(time.Hour)
	defer cleanupTimer.Stop()

	connectionListener := channel.Map.GetSockMap(r.ID).Listen()
	defer channel.Map.GetSockMap(r.ID).UnListen(connectionListener)

	for {
		select {
		// handle move events
		case move := <-r.moveChannel:
			util.DebugFlag("room", str.CRoom, "[%s] got move %s from %s (%s / %s)", r.ID, move.Move.Uoi, move.Player, r.game.White, r.game.Black)
			moveStart := time.Now()
			// if not player's turn, send previous position and continue
			if !r.isTurn(move) {
				channel.Unicast(r.GetSerializedGameState(), move.Ctx)
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

				// stop cleanup timer
				cleanupTimer.Stop()

				return
			}

			// track move lag for later compensation
			go lag.Move.Track(moveStart)
			util.DebugFlag("lag", str.CRoom, "move lag avg: %s", lag.Move.Get())

		// handle clock events
		case flaggedState := <-r.game.Clock.StateChannel:
			util.DebugFlag("room", str.CRoom, "[%s] flagged clock state: %+v", flaggedState)
			// automatically resign game if clock expires
			if flaggedState.Victor == clock.White {
				r.game.Resign(octad.Black)
			} else {
				r.game.Resign(octad.White)
			}

			// run game over routine and get transition event type
			_, event := r.tryGameOver(channel.SocketContext{Channel: r.ID, MT: 2}, false)

			// make state transition and exit the gameOngoing routine
			err := r.event(*event)
			if err != nil {
				panic(err)
			}

			// stop cleanup timer
			cleanupTimer.Stop()

			return
		// handle start/stop of abandon timer when players connect and disconnect
		case numConnections := <-connectionListener:
			util.DebugFlag("room", str.CRoom, "[%s] room player count changed: %d", r.ID, numConnections)
			if r.BothPlayersConnected() {
				util.DebugFlag("room", str.CRoom, "[%s] both players connected, cancelling abandon timer", r.ID)
				if cleanupTimer != nil {
					cleanupTimer.Stop()
				}
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] players not connected, starting abandon timer", r.ID)

			// start cleanup timer if both players are not connected
			if cleanupTimer == nil {
				cleanupTimer = time.NewTimer(abandonTimeout)
			} else {
				cleanupTimer.Reset(abandonTimeout)
			}
		// figure out who abandoned and resign the game
		case <-cleanupTimer.C:
			// if both players abandoned
			if !r.BothPlayersConnected() {
				util.DebugFlag("room", str.CRoom, "[%s] both players abandoned, game drawn", r.ID)
				// draw the game immediately
				err := r.game.Draw(octad.DrawOffer)
				if err != nil {
					panic(err)
				}
			} else {
				connected := r.GetConnectedPlayers()
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
			r.tryGameOver(channel.SocketContext{Channel: r.ID, MT: 2}, true)

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
