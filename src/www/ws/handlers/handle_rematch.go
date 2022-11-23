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

// HandleRematch processes rematch request messages
func HandleRematch(m []byte, meta channel.SocketContext) []byte {
	thisRoom, err := room.Get(meta.RoomID)
	if err != nil {
		return nil
	}

	var msg proto.MessageRematch
	err = json.Unmarshal(m, &msg)
	if err != nil {
		util.Error(str.CHMov, str.ERematchUnmarshal, m, err)
		return nil
	}

	util.Debug(str.CRoom, "rematch message %+v", msg)

	// send rematch request to room on player request
	if msg.Data.PlayerRequest {
		util.Debug(str.CRoom, "sending rematch")
		thisRoom.SendControl(&message.RoomControl{
			Player: meta.UID,
			Type:   message.Rematch,
			Ctx:    meta,
		})
	}

	return nil
}
