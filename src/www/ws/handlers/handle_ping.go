package handlers

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"google.golang.org/protobuf/proto"
)

// HandlePing processes ping messages from the client
func HandlePing() []byte {
	pong := wsv1.WebsocketMessage{
		Data: &wsv1.WebsocketMessage_PongPayload{
			PongPayload: &wsv1.PongPayload{},
		},
	}

	payload, err := proto.Marshal(&pong)
	if err != nil {
		util.Error(str.CChan, "error encoding pong message err=%s", err.Error())
		return nil
	}

	//util.DebugFlag("ws", str.CWS, str.DWSSend, pong.Data)

	return payload
}
