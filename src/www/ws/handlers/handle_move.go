package handlers

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// HandleMove processes game update messages
func HandleMove(payload *wsv1.MovePayload, meta channel.SocketContext) []byte {
	thisRoom, err := room.Get(meta.RoomID)
	if err != nil {
		util.DebugFlag("h-move", str.CHMov, "no room with id: %s", meta.RoomID)
		return nil
	}

	// send move to room
	thisRoom.SendMove(&message.RoomMove{
		Player: meta.UID, Move: payload, Ctx: meta,
	})

	return nil
}
