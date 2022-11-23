package proto

// Marshal fully JSON marshals the RematchPayload and
// Wraps it in a Message struct
func (m *RematchPayload) Marshal() []byte {
	message := Message{
		Tag:  string(RematchTag),
		Data: m,
	}

	return message.Please()
}
