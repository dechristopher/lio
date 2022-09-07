package channel

import (
	"errors"

	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// Unicast sends an ad-hoc message to the channel and socket that
// the message originated from
func Unicast(d []byte, meta SocketContext) {
	socket := Map.GetSocket(meta.Channel, meta.UID)

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
	for uid := range Map.GetSockMap(meta.Channel).sockets {
		meta.UID = uid
		Unicast(d, meta)
	}
}

// BroadcastEx sends a message to all connected sockets within the
// channel that this message originated from except the originator
func BroadcastEx(d []byte, meta SocketContext) {
	for uid := range Map.GetSockMap(meta.Channel).sockets {
		if uid != meta.UID {
			Unicast(d, SocketContext{
				Channel: meta.Channel,
				UID:     uid,
				MT:      meta.MT,
			})
		}
	}
}
