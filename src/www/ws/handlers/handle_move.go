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

	// quickly return current state on new connection / board-update request
	if fastjson.GetInt(m, "d", "a") == 0 {
		// during the blind deploy phase a (re)connecting client must enter
		// deploy mode, not render the stale pre-deploy board
		if thisRoom.State() == room.StateDeploy {
			return thisRoom.DeployStateMessage(meta.UID)
		}
		// a client returning to a finished game (a refresh, or reopening the tab
		// during the rematch window) must re-enter the game-over overlay rather
		// than resume live play with the clocks running. Send the authoritative
		// final board first so the position renders, then the game-over payload so
		// the overlay shows and the clocks stop — the same two-message sequence a
		// live finish produces (see tryGameOver). GameOverStateMessage returns nil
		// if the game isn't actually over, so this falls through to the plain board
		// state for any other state.
		if thisRoom.State() == room.StateGameOver {
			if overMsg := thisRoom.GameOverStateMessage(); overMsg != nil {
				channel.Unicast(thisRoom.CurrentGameStateMessage(true, false), meta)
				return overMsg
			}
		}
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
