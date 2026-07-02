package proto

import (
	"github.com/dechristopher/lio/channel"
)

// Marshal fully JSON marshals the DrawOfferPayload and
// wraps it in a Message struct
func (d *DrawOfferPayload) Marshal() []byte {
	message := Message{
		Tag:  string(DrawOfferTag),
		Data: d,
	}

	return message.Please()
}

// Broadcast will send a DrawOffer message to all sockets connected
// to the channel within the meta given
func (d DrawOfferPayload) Broadcast(meta channel.SocketContext) {
	channel.Broadcast(d.Marshal(), meta)
}
