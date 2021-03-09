package proto

// Marshal fully JSON marshals the GameOverPayload and
// Wraps it in a Message struct
func (g *GameOverPayload) Marshal() []byte {
	message := Message{
		Tag:  "g",
		Data: g,
	}

	return message.Please()
}
