package handlers

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// HandleGameState returns the current game state
func HandleGameState(meta channel.SocketContext) []byte {
	thisRoom, err := room.Get(meta.RoomID)
	if err != nil {
		util.DebugFlag("h-move", str.CHMov, "no room with id: %s", meta.RoomID)
		return nil
	}

	return thisRoom.GetSerializedGameState()
}
