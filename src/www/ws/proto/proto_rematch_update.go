package proto

import (
	"github.com/dechristopher/lio/channel"
)

// Marshal fully JSON marshals the RematchUpdatePayload and
// Wraps it in a Message struct
func (r *RematchUpdatePayload) Marshal() []byte {
	message := Message{
		Tag:  string(RematchUpdateTag),
		Data: r,
	}

	return message.Please()
}

// Broadcast will send a RematchUpdate message to all sockets connected
// to the channel within the meta given
func (r RematchUpdatePayload) Broadcast(meta channel.SocketContext) {
	channel.Broadcast(r.Marshal(), meta)
}
