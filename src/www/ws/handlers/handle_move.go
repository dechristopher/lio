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
	thisRoom, err := room.Get(meta.Channel)
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

	// send move to room
	thisRoom.SendMove(&message.RoomMove{
		Player: meta.BID, Move: msg.Data, Ctx: meta,
	})

	return nil
}
