package proto

// Marshal fully JSON marshals the MovePayload and
// Wraps it in a Message struct
func (m *RedirectMessage) Marshal() []byte {
	message := Message{
		Tag:  string(RedirectTag),
		Data: m,
	}

	return message.Please()
}
