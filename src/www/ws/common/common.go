package common

import (
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

// Unimplemented is a placeholder for routing unimplemented handler functions
func Unimplemented(proto.Message) proto.Message {
	return proto.Message{Error: "unimplemented handler"}
}

// GenResponse generates a base response message from an incoming message
func GenResponse(m proto.Message) proto.Message {
	message := proto.Message{}
	message.Time = util.MilliTime()
	message.Command = m.Command
	message.Body = make([]string, 0)
	return message
}
