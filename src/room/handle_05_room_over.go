package room

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/octad"
	"google.golang.org/protobuf/proto"

	"github.com/dechristopher/lio/channel"
)

// handleGameOver handles room finalization and player notification
func (r *Instance) handleRoomOver() {
	// send game over message if match expired
	if r.abandoned && r.game.Outcome() == octad.NoOutcome {
		websocketMessage := wsv1.WebsocketMessage{Data: &wsv1.WebsocketMessage_GameOverPayload{GameOverPayload: &wsv1.GameOverPayload{
			Status:   "Match expired. Leaving room..",
			RoomOver: true,
		}}}

		payload, err := proto.Marshal(&websocketMessage)
		if err != nil {
			util.Error(str.CChan, "error encoding game over message err=%s", err.Error())
		}

		channel.Broadcast(payload, channel.SocketContext{
			Channel: r.ID,
			MT:      2,
		})
	}
}
