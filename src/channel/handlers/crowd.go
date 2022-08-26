package handlers

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// HandleCrowd monitors ChanMap on a channel and emits crowd message
// broadcasts to everyone in the channel
func HandleCrowd(thisChannel string) {
	meta := channel.SocketContext{
		Channel: thisChannel,
		MT:      1,
	}
	var spectators int
	// range over channel entries until it is closed, then exit routine
	for spectators = range channel.Map[thisChannel].Listen() {
		util.DebugFlag("crowd", str.CChan, "spec: %d", spectators)
		proto.CrowdPayload{
			Spec: spectators,
		}.Broadcast(meta)
	}

	util.DebugFlag("crowd", str.CChan, "cleanup: %s", thisChannel)
}
