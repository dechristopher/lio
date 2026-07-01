package handlers

import (
	"encoding/json"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// HandleDeploy processes blind deploy-phase submissions: a player's home-rank
// ordering is forwarded to the room, which validates it and reveals the board
// once both players (or the deploy timer) are in.
func HandleDeploy(m []byte, meta channel.SocketContext) []byte {
	thisRoom, err := room.Get(meta.RoomID)
	if err != nil {
		return nil
	}

	var msg proto.MessageDeploy
	if err := json.Unmarshal(m, &msg); err != nil {
		util.Error(str.CHMov, "deploy unmarshal error: %s %v", m, err)
		return nil
	}

	util.DebugFlag("room", str.CHMov, "[%s] deploy %q received from %s", meta.RoomID, msg.Data.Order, meta.UID)

	thisRoom.SubmitDeploy(&message.RoomDeploy{
		Player: meta.UID,
		Order:  msg.Data.Order,
		Ctx:    meta,
	})

	return nil
}
