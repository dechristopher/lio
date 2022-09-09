package proto

// Marshal fully JSON marshals the MovePayload and
// Wraps it in a Message struct
func (m *RoomMessage) Marshal() []byte {
	message := Message{
		Tag:  string(RoomTag),
		Data: m,
	}

	return message.Please()
}
