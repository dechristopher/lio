package room

import (
	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

// handleRoomOver handles room finalization and player notification. It tells
// clients to leave the room so they aren't left on a frozen final board.
func (r *Instance) handleRoomOver() {
	r.stateMu.Lock()
	gameFinished := r.game.Outcome() != octad.NoOutcome
	r.stateMu.Unlock()

	var status string
	switch {
	case r.abandoned && !gameFinished:
		// the room expired before any game finished
		status = "Match expired. Leaving room.."
	case !r.abandoned:
		// a finished game closed without a rematch: a human-vs-human window that
		// lapsed, or a finished bot game the player left (after the reconnect grace)
		status = "No rematch. Leaving room.."
	default:
		// abandoned after a game finished: the game-over broadcast already
		// carried RoomOver=true, so clients are already leaving
		return
	}

	payload := proto.GameOverPayload{
		Status:   status,
		RoomOver: true,
	}

	channel.Broadcast(payload.Marshal(), channel.SocketContext{
		Channel: r.ID,
		MT:      1,
	})
}
