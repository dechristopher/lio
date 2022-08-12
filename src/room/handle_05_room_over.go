package room

import (
	"github.com/dechristopher/lioctad/channel"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

// handleGameOver handles room finalization and player notification
func (r *Instance) handleRoomOver() {
	payload := proto.GameOverPayload{
		Status:   "Rematch denied, or match abandoned. Leaving room..",
		RoomOver: true,
	}
	channel.Broadcast(payload.Marshal(), channel.SocketContext{
		Channel: r.ID,
		MT:      1,
	})
}
