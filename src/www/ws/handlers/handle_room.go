package handlers

import (
	"encoding/json"
	"fmt"

	"github.com/valyala/fastjson"

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

	// rematch / resign / draw are controls, not state queries; read them from
	// the message payload directly (like HandleMove) and hand them to the room.
	// Spectator frames skip the whole control block — only seated players hold
	// game controls (the room's own seat checks remain as a second layer) — but
	// fall through to the readiness query below, which spectators may use.
	if !meta.IsSpectator {
		if fastjson.GetBool(m, "d", "rm") {
			thisRoom.RequestRematch(meta)
			return nil
		}
		if fastjson.GetBool(m, "d", "rs") {
			thisRoom.RequestResign(meta)
			return nil
		}
		if fastjson.GetBool(m, "d", "dr") {
			thisRoom.RequestDraw(meta)
			return nil
		}
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
