package proto

// Pong returns a new PongMessage body
func Pong() []byte {
	return []byte(`{"po": 1}`)
}
