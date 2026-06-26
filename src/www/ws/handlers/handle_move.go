package handlers

import (
	"encoding/json"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/room"
	"github.com/valyala/fastjson"

	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// HandleMove processes game update messages
func HandleMove(m []byte, meta channel.SocketContext) []byte {
	thisRoom, err := room.Get(meta.RoomID)
	if err != nil {
		return nil
	}

	// quickly return board state on new connection
	if fastjson.GetInt(m, "d", "a") == 0 {
		return thisRoom.CurrentGameStateMessage(true, false)
	}

	var msg proto.MessageMove
	err = json.Unmarshal(m, &msg)
	if err != nil {
		util.Error(str.CHMov, str.EMoveUnmarshal, m, err)
		return thisRoom.CurrentGameStateMessage(false, false)
	}

	// trace inbound moves so a move that reached the server can be told apart
	// from one lost in transit (the client never logs an unconfirmed move).
	// Pair this with the SendMove drop log to follow a move's full lifecycle
	// under `--debug room`.
	util.DebugFlag("room", str.CHMov, "[%s] move %s received from %s", meta.RoomID, msg.Data.UOI, meta.UID)

	// send move to room
	thisRoom.SendMove(&message.RoomMove{
		Player: meta.UID, Move: msg.Data, Ctx: meta,
	})

	return nil
}
