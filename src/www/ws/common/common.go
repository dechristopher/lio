package common

import (
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// Unimplemented is a placeholder for routing unimplemented handler functions
func Unimplemented(_ []byte, _ SocketMeta) []byte {
	return []byte("{\"ok\": false}")
}

// Unicast sends an ad-hoc message to the channel and socket that
// the message originated from
func Unicast(d []byte, meta SocketMeta) {
	meta.Sockets[meta.Channel][meta.BID].Mutex.Lock()
	err := meta.Sockets[meta.Channel][meta.BID].Connection.
		WriteMessage(meta.MT, d)
	if err != nil {
		util.Error(str.CWSC, str.EWSWrite, meta, err)
	}
	meta.Sockets[meta.Channel][meta.BID].Mutex.Unlock()
}

// Broadcast sends a message to all connected sockets within the
// channel that this message originated from
func Broadcast(d []byte, meta SocketMeta) {
	util.Debug(str.CHMov, "broadcast")
	for bid, _ := range meta.Sockets[meta.Channel] {
		meta.BID = bid
		go Unicast(d, meta)
	}
}

// BroadcastEx sends a message to all connected sockets within the
// channel that this message originated from except the originator
func BroadcastEx(d []byte, meta SocketMeta) {
	for bid, _ := range meta.Sockets[meta.Channel] {
		if bid != meta.BID {
			go Unicast(d, SocketMeta{
				Sockets: meta.Sockets,
				Channel: meta.Channel,
				BID:     bid,
				MT:      meta.MT,
			})
		}
	}
}
