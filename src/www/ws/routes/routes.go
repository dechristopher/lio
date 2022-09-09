package routes

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/www/ws/handlers"
	"github.com/dechristopher/lio/www/ws/proto"
)

// wsRoutes is a type that tracks websocket handlers and the
// message types that they correspond with
type wsRoutes = map[proto.PayloadTag]channel.Handler

// Map protocol commands to command handlers
var (
	Map = wsRoutes{
		proto.MoveTag: handlers.HandleMove,
		proto.RoomTag: handlers.HandleRoom,
		proto.OFENTag: Unimplemented,
	}
)

// Unimplemented is a placeholder for routing unimplemented handler functions
func Unimplemented(_ []byte, _ channel.SocketContext) []byte {
	return []byte("{\"ok\": false}")
}
