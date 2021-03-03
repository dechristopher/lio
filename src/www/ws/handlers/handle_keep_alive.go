package handlers

import (
	"github.com/dechristopher/lioctad/www/ws/common"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

// HandleKeepAlive responds to socket CommandKeepAlive messages
func HandleKeepAlive(m proto.Message) proto.Message {
	response := common.GenResponse(m)
	response.Body = []string{"ok"}
	return response
}
