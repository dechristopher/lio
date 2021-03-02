package routes

import (
	"github.com/dechristopher/lioctad/www/ws/common"
	"github.com/dechristopher/lioctad/www/ws/handlers"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

// Map protocol commands to command handlers
var (
	Map = map[int]func(m proto.Message) proto.Message{
		proto.CommandError:   common.Unimplemented,
		proto.CommandGoodbye: common.Unimplemented,
		proto.CommandHello:   handlers.HandleHello,
		proto.CommandGame:    handlers.HandleGame,
	}
)
