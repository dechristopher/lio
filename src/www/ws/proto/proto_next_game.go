package proto

import (
	"github.com/dechristopher/lio/channel"
)

// Marshal fully JSON marshals the NextGamePayload and
// Wraps it in a Message struct
func (n *NextGamePayload) Marshal() []byte {
	message := Message{
		Tag:  string(NextGameTag),
		Data: n,
	}

	return message.Please()
}

// Broadcast will send a NextGame message to all sockets connected
// to the channel within the meta given
func (n NextGamePayload) Broadcast(meta channel.SocketContext) {
	channel.Broadcast(n.Marshal(), meta)
}
