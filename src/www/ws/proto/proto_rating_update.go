package proto

// Marshal fully JSON marshals the RatingUpdatePayload and wraps it in a Message.
func (r *RatingUpdatePayload) Marshal() []byte {
	message := Message{
		Tag:  string(RatingUpdateTag),
		Data: r,
	}

	return message.Please()
}
