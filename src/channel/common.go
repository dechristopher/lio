package channel

import (
	"errors"

	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// Unicast sends an ad-hoc message to the channel and socket that
// the message originated from
func Unicast(d []byte, meta SocketContext) {
	socket := Map[meta.Channel].Get(meta.BID)

	if socket == nil || socket.Mutex == nil {
		util.Error(str.CWSC, str.EWSWrite, meta, errors.New("socket nil"))
		return
	}

	socket.Mutex.Lock()
	defer socket.Mutex.Unlock()
	err := socket.Connection.WriteMessage(meta.MT, d)
	if err != nil {
		util.Error(str.CWSC, str.EWSWrite, meta, err)
	}
}

// Broadcast sends a message to all connected sockets within the
// channel that this message originated from
func Broadcast(d []byte, meta SocketContext) {
	for bid := range Map[meta.Channel].sockets {
		meta.BID = bid
		Unicast(d, meta)
	}
}

// BroadcastEx sends a message to all connected sockets within the
// channel that this message originated from except the originator
func BroadcastEx(d []byte, meta SocketContext) {
	for bid := range Map[meta.Channel].sockets {
		if bid != meta.BID {
			Unicast(d, SocketContext{
				Channel: meta.Channel,
				BID:     bid,
				MT:      meta.MT,
			})
		}
	}
}
