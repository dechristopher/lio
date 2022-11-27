package proto

// Marshal fully JSON marshals the RoomPayload and
// Wraps it in a Message struct
func (m *RoomPayload) Marshal() []byte {
	message := Message{
		Tag:  string(RoomTag),
		Data: m,
	}

	return message.Please()
}
