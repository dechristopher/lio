package proto

import "github.com/dechristopher/lioctad/www/ws/common"

// Marshal fully JSON marshals the CrowdPayload and
// Wraps it in a Message struct
func (c *CrowdPayload) Marshal() []byte {
	message := Message{
		Tag:  "c",
		Data: c,
	}

	return message.Please()
}

// Broadcast will send a Crowd message to all sockets connected
// to the channel within the meta given
func (c CrowdPayload) Broadcast(meta common.SocketContext) {
	common.Broadcast(c.Marshal(), meta)
}
