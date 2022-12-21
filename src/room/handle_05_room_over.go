package room

import (
	"fmt"
	"github.com/dechristopher/lio/channel"
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"google.golang.org/protobuf/proto"
)

// handleGameOver handles room finalization and player notification
func (r *Instance) handleRoomOver() {
	// TODO would a "RoomOver" message make more sense than a redirect?
	websocketMessage := wsv1.WebsocketMessage{Data: &wsv1.WebsocketMessage_RedirectPayload{RedirectPayload: &wsv1.RedirectPayload{
		Location: "/",
	}}}

	payload, err := proto.Marshal(&websocketMessage)
	if err != nil {
		util.Error(str.CChan, "error encoding redirect message err=%s", err.Error())
	}
	// broadcast a redirect to every room channel
	for _, channelType := range roomChannelTypes {
		channel.Broadcast(payload, channel.SocketContext{
			Channel: fmt.Sprintf("%s%s", channelType, r.ID),
			MT:      2,
		})
	}
}
