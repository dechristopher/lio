package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// HandleRoom processes rom update request messages
func HandleRoom(m []byte, meta channel.SocketContext) []byte {
	thisRoom, err := room.Get(meta.RoomID)
	if err != nil {
		return nil
	}

	var msg proto.RoomMessage
	err = json.Unmarshal(m, &msg)
	if err != nil {
		util.Error(str.CHMov, str.EMoveUnmarshal, m, err)
		return nil
	}

	// if requesting room readiness update, return redirect message
	// if the room is ready for gameplay
	if msg.Query {
		util.Info("HR", "queried")
		if thisRoom.IsReady() {
			util.Info("HR", "isReady")
			redirectMessage := proto.RedirectMessage{
				Location: fmt.Sprintf("/%s", thisRoom.ID),
			}
			return redirectMessage.Marshal()
		}
	}

	return nil
}
