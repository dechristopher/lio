package channel

import (
	"sync"
	"time"

	"github.com/gofiber/websocket/v2"
)

type Handler = func(m []byte, meta SocketContext) []byte

// SockMap tracks every Socket connected to a given channel, keyed by uid and
// then by per-connection id. Storing a set of sockets per uid (rather than a
// single socket) means a user can hold several connections at once (multiple
// tabs, or independent multimodal streams) and that a stale connection's
// teardown can only ever evict itself — never a newer, live connection for the
// same uid (the old reconnect "ghost-disconnect" bug).
type SockMap struct {
	sockets map[string]map[string]*Socket // uid -> connID -> socket
	channel string

	mut *sync.Mutex

	// updateChannel is a coalescing "dirty" signal: Track/UnTrack post to it
	// (non-blocking) whenever the connected-user set changes, and the
	// broadcaster goroutine recomputes the current count and forwards it to
	// listeners. It is buffered (cap 1) and is NEVER closed, so a post can never
	// panic on a closed channel even if it races Cleanup.
	updateChannel chan struct{}

	// listenChannels are the registered listener channels. They are only ever
	// closed under mut (by UnListen or Cleanup), and the broadcaster only sends
	// to them under mut, so a send can never hit a closed channel.
	listenChannels []chan int

	// cleanup is closed (once) by Cleanup to stop the broadcaster goroutine.
	cleanup chan struct{}
	// closed guards against double-cleanup and tells Listen to hand back an
	// already-closed channel after the SockMap has been torn down.
	closed bool
}

// NewSockMap returns a new SockMap for the given channel
func NewSockMap(channel string) *SockMap {
	s := &SockMap{
		sockets: make(map[string]map[string]*Socket),
		channel: channel,

		mut: &sync.Mutex{},

		updateChannel:  make(chan struct{}, 1),
		listenChannels: make([]chan int, 0),

		cleanup: make(chan struct{}),
	}

	go s.broadcastToListeners()

	return s
}

// Track adds the given socket to the internal sockets map (under its uid and
// connection id) and signals the crowd broadcaster that the count changed.
func (s *SockMap) Track(sock *Socket) {
	if s == nil || sock == nil {
		return
	}
	s.mut.Lock()
	conns := s.sockets[sock.UID]
	if conns == nil {
		conns = make(map[string]*Socket)
		s.sockets[sock.UID] = conns
	}
	conns[sock.ID] = sock
	s.mut.Unlock()
	s.notify()
}

// UnTrack removes a single connection (uid + connID) from the internal sockets
// map and signals the crowd broadcaster that the count changed. The uid is
// dropped only once its last connection goes away.
func (s *SockMap) UnTrack(uid, connID string) {
	if s == nil {
		return
	}
	s.mut.Lock()
	if conns := s.sockets[uid]; conns != nil {
		delete(conns, connID)
		if len(conns) == 0 {
			delete(s.sockets, uid)
		}
	}
	s.mut.Unlock()
	s.notify()
}

// notify posts a non-blocking, coalescing "count changed" signal to the
// broadcaster. It never blocks the caller (Track/UnTrack run on connection
// goroutines) and never panics: updateChannel is buffered and never closed.
func (s *SockMap) notify() {
	select {
	case s.updateChannel <- struct{}{}:
	default:
		// a refresh is already pending; the broadcaster will read the
		// up-to-date count when it services it
	}
}

// Connected reports whether the given uid currently holds at least one live
// connection on this channel. This is the player-presence primitive used by the
// room's abandon detection.
func (s *SockMap) Connected(uid string) bool {
	if s == nil {
		return false
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	return len(s.sockets[uid]) > 0
}

// Sockets returns a snapshot of every connection tracked on this channel.
// Broadcasts range over this snapshot rather than the live map so they neither
// hold the SockMap lock across enqueues nor race concurrent Track/UnTrack.
func (s *SockMap) Sockets() []*Socket {
	if s == nil {
		return nil
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	out := make([]*Socket, 0, len(s.sockets))
	for _, conns := range s.sockets {
		for _, sock := range conns {
			out = append(out, sock)
		}
	}
	return out
}

// SocketsFor returns a snapshot of every connection a single uid holds on this
// channel (e.g. all of a user's open tabs).
func (s *SockMap) SocketsFor(uid string) []*Socket {
	if s == nil {
		return nil
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	conns := s.sockets[uid]
	out := make([]*Socket, 0, len(conns))
	for _, sock := range conns {
		out = append(out, sock)
	}
	return out
}

// Empty returns true if the SockMap is tracking no connected users
func (s *SockMap) Empty() bool {
	return s.Length() == 0
}

// Cleanup cleans up all SockMap resources. It is safe to call more than once.
func (s *SockMap) Cleanup() {
	if s == nil {
		return
	}

	s.mut.Lock()
	if s.closed {
		s.mut.Unlock()
		return
	}
	s.closed = true
	// signal every tracked connection's writer to shut down so the underlying
	// websockets close and their read loops unwind
	for _, conns := range s.sockets {
		for _, sock := range conns {
			sock.Close()
		}
	}
	// close all listener channels so consumers' ranges/selects exit. This is
	// the only place (besides UnListen) that closes them, and it runs under
	// mut, so it can never double-close or race the broadcaster's sends.
	for i := range s.listenChannels {
		if s.listenChannels[i] != nil {
			close(s.listenChannels[i])
			s.listenChannels[i] = nil
		}
	}
	s.mut.Unlock()

	// stop the broadcaster goroutine
	close(s.cleanup)

	Map.Delete(s.channel)
}

// Length returns the number of distinct connected users (not connections). A
// single user with several open tabs counts once, which is what the crowd
// count and the room's player-count transitions expect.
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
	if s == nil {
		return
	}
	s.mut.Lock()
	defer s.mut.Unlock()
	for i := range s.listenChannels {
		if s.listenChannels[i] == listener {
			close(s.listenChannels[i])
			s.listenChannels[i] = nil
			return
		}
	}
}

// Listen returns a new channel for listening to connected-user count updates.
// The returned channel is buffered (cap 1): listeners treat each receive as a
// wakeup signal and re-derive truth from the live SockMap, so a coalesced
// (dropped intermediate) update is fine as long as one wakeup always follows
// the last change — which the buffer guarantees.
func (s *SockMap) Listen() Listener {
	if s == nil {
		return nil
	}

	listener := make(chan int, 1)

	s.mut.Lock()
	defer s.mut.Unlock()

	// SockMap already torn down: hand back a closed channel so a ranging
	// consumer exits immediately instead of blocking forever.
	if s.closed {
		close(listener)
		return listener
	}

	s.listenChannels = append(s.listenChannels, listener)
	// prime with the current count (buffered, so this never blocks)
	listener <- len(s.sockets)

	return listener
}

// broadcastToListeners forwards connected-user count changes to every
// registered listener until Cleanup stops it.
func (s *SockMap) broadcastToListeners() {
	for {
		select {
		case <-s.updateChannel:
			s.mut.Lock()
			n := len(s.sockets)
			for i := range s.listenChannels {
				lc := s.listenChannels[i]
				if lc == nil {
					continue
				}
				// non-blocking: if the listener hasn't drained its last
				// wakeup yet, skip — it will re-derive current state when it
				// reads the pending value.
				select {
				case lc <- n:
				default:
				}
			}
			s.mut.Unlock()
		case <-s.cleanup:
			return
		}
	}
}

// Socket wraps a single websocket connection. All writes to the connection are
// owned by its WritePump goroutine; other goroutines hand messages off via
// Enqueue, so there is no shared write mutex and a slow/dead client can never
// block a broadcaster.
type Socket struct {
	Connection *websocket.Conn
	ID         string // per-connection id, unique within a uid
	UID        string
	Type       string

	send   chan []byte
	closed chan struct{}
	once   sync.Once
}

// NewSocket builds a tracked connection wrapper with its own send buffer.
func NewSocket(conn *websocket.Conn, uid, connID, typ string) *Socket {
	return &Socket{
		Connection: conn,
		ID:         connID,
		UID:        uid,
		Type:       typ,
		send:       make(chan []byte, SendBuffer),
		closed:     make(chan struct{}),
	}
}

// Enqueue queues a message for the connection's writer goroutine. It never
// blocks: if the send buffer is full the consumer is too slow or wedged, so the
// connection is dropped (its read loop then unwinds and untracks it).
func (s *Socket) Enqueue(d []byte) {
	select {
	case s.send <- d:
	case <-s.closed:
		// already shutting down; drop
	default:
		// buffer full: slow/stuck client — drop the connection
		s.Close()
	}
}

// Close signals the writer goroutine to shut the connection down. Idempotent
// and safe to call from any goroutine.
func (s *Socket) Close() {
	s.once.Do(func() { close(s.closed) })
}

// WritePump owns all writes to the underlying connection: it drains queued
// messages, applies a write deadline to each, and sends periodic protocol-level
// ping frames so a vanished client (one that never sent a TCP FIN) is detected.
// It exits — closing the connection — on any write error, on Close, or when the
// read side tears the connection down.
func (s *Socket) WritePump() {
	ticker := time.NewTicker(PingPeriod)
	defer func() {
		ticker.Stop()
		_ = s.Connection.Close()
	}()

	for {
		select {
		case msg := <-s.send:
			_ = s.Connection.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := s.Connection.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = s.Connection.SetWriteDeadline(time.Now().Add(WriteWait))
			if err := s.Connection.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		case <-s.closed:
			// best-effort clean close handshake before the deferred Close
			_ = s.Connection.SetWriteDeadline(time.Now().Add(WriteWait))
			_ = s.Connection.WriteMessage(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
			return
		}
	}
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
