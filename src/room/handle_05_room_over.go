package room

import (
	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/proto"
)

// handleGameOver handles room finalization and player notification
func (r *Instance) handleRoomOver() {
	var payload proto.GameOverPayload

	// send game over message if match expired
	if r.abandoned && r.game.Outcome() == octad.NoOutcome {
		payload = proto.GameOverPayload{
			StatusID: proto.OverAbandoned,
			RoomOver: true,
		}
	} else {
		payload = proto.GameOverPayload{
			StatusID: proto.OverNoRematch,
			RoomOver: true,
		}
	}

	channel.Broadcast(payload.Marshal(), channel.SocketContext{
		Channel: r.ID,
		MT:      1,
	})
}
