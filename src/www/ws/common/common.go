package common

import (
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// Unimplemented is a placeholder for routing unimplemented handler functions
func Unimplemented(_ []byte, _ SocketContext) []byte {
	return []byte("{\"ok\": false}")
}

// Unicast sends an ad-hoc message to the channel and socket that
// the message originated from
func Unicast(d []byte, meta SocketContext) {
	meta.Sockets[meta.Channel].Get(meta.BID).Mutex.Lock()
	defer meta.Sockets[meta.Channel].Get(meta.BID).Mutex.Unlock()
	err := meta.Sockets[meta.Channel].Get(meta.BID).
		Connection.WriteMessage(meta.MT, d)
	if err != nil {
		util.Error(str.CWSC, str.EWSWrite, meta, err)
	}
}

// Broadcast sends a message to all connected sockets within the
// channel that this message originated from
func Broadcast(d []byte, meta SocketContext) {
	for bid := range meta.Sockets[meta.Channel].sockets {
		meta.BID = bid
		go Unicast(d, meta)
	}
}

// BroadcastEx sends a message to all connected sockets within the
// channel that this message originated from except the originator
func BroadcastEx(d []byte, meta SocketContext) {
	for bid := range meta.Sockets[meta.Channel].sockets {
		if bid != meta.BID {
			go Unicast(d, SocketContext{
				Sockets: meta.Sockets,
				Channel: meta.Channel,
				BID:     bid,
				MT:      meta.MT,
			})
		}
	}
}
