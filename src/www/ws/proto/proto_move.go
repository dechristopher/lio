package proto

// Marshal fully JSON marshals the MovePayload and
// Wraps it in a Message struct
func (m *MovePayload) Marshal() []byte {
	message := Message{
		Tag:          "m",
		Data:         m,
		ProtoVersion: MovePayloadVersion,
	}

	return message.Please()
}
