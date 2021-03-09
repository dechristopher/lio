package routes

import (
	"github.com/dechristopher/lioctad/www/ws/common"
	"github.com/dechristopher/lioctad/www/ws/handlers"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

// wsRoutes is a type that tracks websocket handlers and the
// message types that they correspond with
type wsRoutes = map[proto.PayloadTag]func(m []byte, meta common.SocketMeta) []byte

// Map protocol commands to command handlers
var (
	Map = wsRoutes{
		proto.MoveTag: handlers.HandleMove,
		proto.OFENTag: common.Unimplemented,
	}
)
