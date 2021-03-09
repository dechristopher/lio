package proto

import "github.com/dechristopher/lioctad/www/ws/common"

// Marshal fully JSON marshals the MovePayload and
// Wraps it in a Message struct
func (c *CrowdPayload) Marshal() []byte {
	message := Message{
		Tag:  "c",
		Data: c,
	}

	return message.Please()
}

func (c CrowdPayload) Broadcast(meta common.SocketMeta) {
	common.Broadcast(c.Marshal(), meta)
}
