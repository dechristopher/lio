package proto

import (
	"encoding/json"

	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// Marshal fully JSON marshals the GameOverPayload and
// Wraps it in a Message struct
func (g *GameOverPayload) Marshal() []byte {
	message := Message{
		Tag:  "g",
		Data: g,
	}

	b, err := json.Marshal(&message)

	if err != nil {
		util.Error(str.CWSC, str.EWSWrite, g, err)
		return nil
	}

	return b
}
