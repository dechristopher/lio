package handlers

import (
	"strings"

	"github.com/valyala/fastjson"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/home"
)

// maxWatchRoomID bounds an inbound watch target. Room ids are short generated
// codes, so anything longer is malformed — and the id becomes a map key in the
// hub's watch registry, which is reason enough not to take one on trust.
const maxWatchRoomID = 64

// HandleWatch registers (or cancels) this connection's standing watch on one
// room's live game — the username hover card's mini board
// (arch/PLAYER_CARD.md). An empty room id cancels.
//
// It is a read-only subscription and is deliberately open to spectators and
// anonymous sessions alike: everything it can ever deliver is the same TVGame
// the home page broadcasts to everybody, so there is nothing here a viewer
// could not already see by opening the room.
//
// The reply is pushed by the hub rather than returned, because every later
// frame arrives that way too and one path is easier to reason about than two.
func HandleWatch(m []byte, meta channel.SocketContext) []byte {
	roomID := strings.TrimSpace(fastjson.GetString(m, "d", "r"))
	if len(roomID) > maxWatchRoomID {
		return nil
	}
	home.Watch(meta.Channel, meta.UID, roomID)
	return nil
}
