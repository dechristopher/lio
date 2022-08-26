package room

import (
	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

// handleGameOver handles room finalization and player notification
func (r *Instance) handleRoomOver() {
	// send game over message if match expired
	if r.abandoned && r.game.Outcome() == octad.NoOutcome {
		payload := proto.GameOverPayload{
			Status:   "Match expired. Leaving room..",
			RoomOver: true,
		}

		channel.Broadcast(payload.Marshal(), channel.SocketContext{
			Channel: r.ID,
			MT:      1,
		})
	}
}
