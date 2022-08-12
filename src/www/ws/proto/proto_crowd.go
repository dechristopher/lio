package proto

import (
	"github.com/dechristopher/lioctad/channel"
)

// Marshal fully JSON marshals the CrowdPayload and
// Wraps it in a Message struct
func (c *CrowdPayload) Marshal() []byte {
	message := Message{
		Tag:  string(CrowdTag),
		Data: c,
	}

	return message.Please()
}

// Broadcast will send a Crowd message to all sockets connected
// to the channel within the meta given
func (c CrowdPayload) Broadcast(meta channel.SocketContext) {
	channel.Broadcast(c.Marshal(), meta)
}
