package common

import (
	"sync"

	"github.com/gofiber/websocket/v2"
)

// SockMap tracks every Socket connected to a given channel
type SockMap struct {
	sockets map[string]Socket
	channel string
	C       chan int
}

// NewSockMap returns a new SockMap for the given channel
func NewSockMap(channel string) SockMap {
	return SockMap{
		sockets: make(map[string]Socket),
		channel: channel,
		C:       make(chan int),
	}
}

// Track adds the given socket to the internal sockets map
// and emits an update to the crowd channel
func (s SockMap) Track(bid string, sock Socket) {
	s.sockets[bid] = sock
	s.C <- len(s.sockets)
}

// UnTrack removes the given socket from the internal sockets
// map and emits an update to the crowd channel
func (s SockMap) UnTrack(bid string) {
	delete(s.sockets, bid)
	s.C <- len(s.sockets)
}

// Get returns a given Socket by bid
func (s SockMap) Get(bid string) Socket {
	return s.sockets[bid]
}

// Empty returns true if the SockMap is tracking no connected sockets
func (s SockMap) Empty() bool {
	return s.Length() == 0
}

// Length returns the number of actively connected sockets
func (s SockMap) Length() int {
	return len(s.sockets)
}

// Socket is a struct combining a websocket connection and a mutex lock
// for best practice, protected synchronous reads and writes to websockets
type Socket struct {
	Connection *websocket.Conn
	Mutex      *sync.Mutex
}

// SocketContext contains all relevant information about the message
// data received by a websocket handler
type SocketContext struct {
	Sockets map[string]SockMap
	Channel string
	BID     string
	MT      int
}
