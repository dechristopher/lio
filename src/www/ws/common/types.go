package common

import (
	"sync"

	"github.com/gofiber/websocket/v2"
)

// Socket is a struct combining a websocket connection and a mutex lock
// for best practice, protected synchronous reads and writes to websockets
type Socket struct {
	Connection *websocket.Conn
	Mutex      *sync.Mutex
}

// SocketMeta contains all relevant information about the message
// data received by a websocket handler
type SocketMeta struct {
	Sockets map[string]map[string]Socket
	Channel string
	BID     string
	MT      int
}
