package channel

import "time"

const (
	// WriteWait bounds how long a single websocket write may block before the
	// connection is treated as failed.
	WriteWait = 10 * time.Second
	// PongWait is how long the read side waits for any inbound traffic (an app
	// message, or a pong answering a server ping) before considering the client
	// gone and tearing the connection down.
	PongWait = 60 * time.Second
	// PingPeriod is how often the writer sends a protocol-level ping. It must be
	// shorter than PongWait so a live-but-quiet client is kept fresh.
	PingPeriod = (PongWait * 9) / 10
	// SendBuffer is the per-connection outbound queue depth. A connection that
	// backs up past this is dropped rather than allowed to stall a broadcast.
	SendBuffer = 64
)

// Unicast queues a message for every connection the target uid holds on the
// channel (e.g. all of that user's open tabs). The actual write happens on each
// connection's own writer goroutine, so this never blocks on a slow client.
func Unicast(d []byte, meta SocketContext) {
	for _, sock := range Map.GetSockMap(meta.Channel).SocketsFor(meta.UID) {
		sock.Enqueue(d)
	}
}

// Broadcast queues a message for every connection on the channel.
//
// It ranges over a snapshot (Sockets) rather than the live map, so it neither
// races concurrent Track/UnTrack nor holds the SockMap lock across enqueues.
func Broadcast(d []byte, meta SocketContext) {
	for _, sock := range Map.GetSockMap(meta.Channel).Sockets() {
		sock.Enqueue(d)
	}
}

// BroadcastEx queues a message for every connection on the channel except those
// belonging to the originating uid.
func BroadcastEx(d []byte, meta SocketContext) {
	for _, sock := range Map.GetSockMap(meta.Channel).Sockets() {
		if sock.UID != meta.UID {
			sock.Enqueue(d)
		}
	}
}
