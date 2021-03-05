package proto

import (
	"encoding/json"

	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// Marshal fully JSON marshals the MovePayload and
// Wraps it in a Message struct
func (m *MovePayload) Marshal() []byte {
	message := Message{
		Tag:          "m",
		Data:         m,
		ProtoVersion: MovePayloadVersion,
	}

	b, err := json.Marshal(&message)

	if err != nil {
		util.Error(str.CWSC, str.EWSWrite, m, err)
		return nil
	}

	return b
}
