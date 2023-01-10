package handlers

import (
	"github.com/dechristopher/lio/channel"
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"google.golang.org/protobuf/proto"
)

// HandleCrowd monitors ChanMap on a channel and emits crowd message
// broadcasts to everyone in the channel
func HandleCrowd(thisChannel string) {
	meta := channel.SocketContext{
		Channel: thisChannel,
		MT:      2,
	}
	var connections int
	// range over channel entries until it is closed, then exit routine
	for connections = range channel.Map.GetSockMap(thisChannel).Listen() {
		util.DebugFlag("crowd", str.CChan, "spec: %d", connections)
		websocketMessage := wsv1.WebsocketMessage{Data: &wsv1.WebsocketMessage_CrowdPayload{CrowdPayload: &wsv1.CrowdPayload{Connections: int32(connections)}}}

		payload, err := proto.Marshal(&websocketMessage)
		if err != nil {
			util.Error(str.CChan, "error encoding crowd message err=%s", err.Error())
		}

		channel.Broadcast(payload, meta)
	}

	util.DebugFlag("crowd", str.CChan, "cleanup: %s", thisChannel)
}
