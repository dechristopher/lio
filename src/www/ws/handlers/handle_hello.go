package handlers

import (
	"github.com/dechristopher/lioctad/www/ws/common"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

// HandleHello spits out default state if any for the
// channel the socket is connected to
func HandleHello(m proto.Message) proto.Message {
	response := common.GenResponse(m)
	//TODO grab state and fill in body
	response.Body = []string{"nice"}
	return response
}
