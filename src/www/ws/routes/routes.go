package routes

import (
	"github.com/dechristopher/lioctad/www/ws/common"
	"github.com/dechristopher/lioctad/www/ws/handlers"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

// Map protocol commands to command handlers
var (
	Map = map[proto.PayloadTag]func(m []byte, meta common.SocketMeta) []byte{
		proto.MoveTag: handlers.HandleMove,
		proto.OFENTag: common.Unimplemented,
	}
)
