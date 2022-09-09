package channel

import (
	"sync"

	"github.com/gofiber/websocket/v2"
)

type Handler = func(m []byte, meta SocketContext) []byte

// SockMap tracks every Socket connected to a given channel
type SockMap struct {
	sockets map[string]*Socket
	channel string

	mut *sync.Mutex

	updateChannel  chan int
	listenChannels []chan int

	cleanup chan bool
}

// NewSockMap returns a new SockMap for the given channel
func NewSockMap(channel string) *SockMap {
	s := &SockMap{
		sockets: make(map[string]*Socket),
		channel: channel,

		mut: &sync.Mutex{},

		updateChannel:  make(chan int),
		listenChannels: make([]chan int, 0),

		cleanup: make(chan bool),
	}

	//TODO go handlers.HandleCrowd(channel)
	go s.broadcastToListeners()

	return s
}

// Track adds the given socket to the internal sockets map
// and emits an update to the crowd channel
func (s *SockMap) Track(uid string, sock *Socket) {
	if s == nil {
		return
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	s.sockets[uid] = sock
	go func() {
		s.updateChannel <- s.Length()
	}()
}

// UnTrack removes the given socket from the internal sockets
// map and emits an update to the crowd channel
func (s *SockMap) UnTrack(uid string) {
	if s == nil {
		return
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	delete(s.sockets, uid)
	go func() {
		s.updateChannel <- s.Length()
	}()
}

// Get returns a given Socket by uid
func (s *SockMap) Get(uid string) *Socket {
	if s == nil {
		return nil
	}
	return s.sockets[uid]
}

// Empty returns true if the SockMap is tracking no connected sockets
func (s *SockMap) Empty() bool {
	return s.Length() == 0
}

// Cleanup cleans up all SockMap resources
func (s *SockMap) Cleanup() {
	s.cleanup <- true

	close(s.updateChannel)

	for i := range s.listenChannels {
		if s.listenChannels[i] != nil {
			close(s.listenChannels[i])
		}
	}

	Map.Delete(s.channel)
}

// Length returns the number of actively connected sockets
func (s *SockMap) Length() int {
	if s == nil {
		return 0
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	return len(s.sockets)
}

type Listener chan int

// UnListen closes a listener channel and un-tracks it from
// the list of listener channels used by the broadcast routine
func (s *SockMap) UnListen(listener Listener) {
	for i := range s.listenChannels {
		if s.listenChannels[i] == listener {
			close(s.listenChannels[i])
			s.listenChannels[i] = nil
			return
		}
	}
}

// Listen returns a new channel for listening to updates
func (s *SockMap) Listen() Listener {
	if s.updateChannel == nil {
		return nil
	}
	listener := make(chan int)
	s.mut.Lock()
	s.listenChannels = append(s.listenChannels, listener)
	s.mut.Unlock()

	// send status immediately
	go func() { listener <- s.Length() }()

	return listener
}

// broadcastToListeners copies updates from socket tracking
// to listener channels registered by external routines
func (s *SockMap) broadcastToListeners() {
	for {
		select {
		case update := <-s.updateChannel:
			for i := range s.listenChannels {
				if s.listenChannels[i] != nil {
					s.listenChannels[i] <- update
				}
			}
		case <-s.cleanup:
			return
		}
	}
}

// Socket is a struct combining a websocket connection and a mutex lock
// for best practice, protected synchronous reads and writes to websockets
type Socket struct {
	Connection *websocket.Conn
	Mutex      *sync.Mutex
	Type       string
}

// SocketContext contains all relevant information about the message
// data received by a websocket handler
type SocketContext struct {
	Channel string
	RoomID  string
	UID     string
	IsBot   bool
	MT      int // websocket message type
}

// IsHuman returns true if the context belongs to a human player
func (ctx *SocketContext) IsHuman() bool {
	return !ctx.IsBot
}
