package channel

import (
	"sync/atomic"
	"time"

	"github.com/gofiber/contrib/v3/websocket"
)

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

// connGen counts every change to the site-wide set of connections: one
// increment per Track and per UnTrack, on every channel.
//
// It is a cheap "has presence changed at all" probe for a reader that would
// otherwise have to walk the whole directory to find out. Connected() is that
// walk, and it costs a snapshot of every SockMap; the home digest wakes on a
// timer and must not pay for the walk when nothing has connected or
// disconnected since it last looked (arch/HOME_ACTIVITY_STREAMING.md).
//
// The value is meaningless in itself — only "same as last time I looked" is.
// It wraps at 2^64 changes, which is not a real number of connections.
var connGen atomic.Uint64

// Generation returns the current connection-change counter. A caller compares
// it against the value it last saw: unchanged means no socket anywhere on the
// site was tracked or untracked in between, so any view derived purely from the
// socket directory is still current.
//
// It says nothing about *room* state, which changes without any connection
// changing. A reader that reflects both must watch both.
func Generation() uint64 {
	return connGen.Load()
}

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

// Connected returns one entry per distinct session uid holding a live socket
// anywhere on the site, mapped to the account that session authenticated as
// (the zero Account for an anonymous visitor).
//
// This is the site-wide presence primitive (package presence). Every page holds
// exactly one socket — the room it is playing in, the wait channel of a
// challenge it created, the home page's TV stream, or the notification channel
// on every other page — so the directory *is* the list of people who are here,
// and no page has to report its presence over HTTP.
//
// Several tabs of one session collapse to one entry: it is one person, and
// every tab carries the same identity. A named record wins over a zero one for
// the same uid, because a session that signs in keeps its uid and its older
// sockets still carry the account it had at upgrade time.
//
// Like the sends below, this ranges over a per-channel snapshot, so it neither
// holds a SockMap's lock across the walk nor races connections starting and
// stopping.
func Connected() map[string]Account {
	out := make(map[string]Account)
	Map.Range(func(_, v interface{}) bool {
		sm, ok := v.(*SockMap)
		if !ok {
			return true
		}
		for _, s := range sm.Sockets() {
			if s.UID == "" {
				continue
			}
			if prev, seen := out[s.UID]; seen && prev.ID != 0 {
				continue
			}
			out[s.UID] = s.Acct
		}
		return true
	})
	return out
}

// SendToAccount queues a message for every connection one signed-in account
// holds, on every channel. It is the delivery primitive for notifications
// (arch/NOTIFICATIONS.md): the sender knows an account, not a session and not a
// channel, so this walks the directory and matches on Socket.Acct.ID.
//
// A person reads the site on several devices and in several tabs. Each of those
// is a separate uid and a separate socket, and each carries its own badge, so
// every one of them gets the frame.
//
// The walk costs one snapshot for each active channel. Notifications are rare —
// a moderation decision, a rating record — so this is cheaper than an index
// from account to socket that Track and UnTrack must maintain correctly. Add
// that index only if this walk ever becomes hot.
//
// Returns the number of connections the message was queued for. A return of 0
// means the account is offline, which is not an error: the row is in the
// database, and the next socket connect reads the count.
// EachSocket calls fn once for every live connection on every channel, with the
// channel name each one is on.
//
// It is the site-wide walk behind anything that has to reach each *connection*
// with a different message — the home hub's follow pushes, which answer a
// per-viewer question and so cannot use the broadcast shape below
// (arch/HOME_ACTIVITY_STREAMING.md).
//
// The channel name comes from the directory key rather than from the Socket,
// which does not carry it. A caller that treats one channel differently from
// the rest (the home page's Following section, which nowhere else renders)
// needs it, and the alternative — a second walk, or a field on every Socket —
// costs more than passing the key already in hand.
//
// Like the sends below it ranges over per-channel snapshots, so it holds no
// SockMap lock across fn and never races connections starting and stopping. fn
// must not block: it runs on the caller's goroutine, part-way through a walk of
// the whole directory.
func EachSocket(fn func(channelName string, s *Socket)) {
	Map.Range(func(k, v interface{}) bool {
		sm, ok := v.(*SockMap)
		if !ok {
			return true
		}
		name, _ := k.(string)
		for _, s := range sm.Sockets() {
			fn(name, s)
		}
		return true
	})
}

func SendToAccount(acctID int64, d []byte) int {
	if acctID == 0 {
		return 0
	}
	sent := 0
	Map.Range(func(_, v interface{}) bool {
		sm, ok := v.(*SockMap)
		if !ok {
			return true
		}
		for _, s := range sm.Sockets() {
			if s.Acct.ID == acctID {
				s.Enqueue(d)
				sent++
			}
		}
		return true
	})
	return sent
}

// SendToAccounts queues a message for every connection held by any of the given
// accounts, in a single walk of the directory. It is the fan-out behind a state
// that belongs to a *group* rather than to one person — the site-wide unread
// feedback count, which every moderator shares (arch/NOTIFICATIONS.md).
//
// One walk rather than a SendToAccount for each id: the set is small, the walk
// is the expensive half, and repeating it per moderator would multiply the cost
// by the size of the staff for no gain.
//
// Returns the number of connections the message was queued for.
func SendToAccounts(ids []int64, d []byte) int {
	if len(ids) == 0 {
		return 0
	}
	want := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		// 0 is the anonymous marker, and every anonymous socket carries it. It
		// must never be a match, or a group message would go to every signed-out
		// visitor on the site.
		if id != 0 {
			want[id] = struct{}{}
		}
	}
	if len(want) == 0 {
		return 0
	}
	sent := 0
	Map.Range(func(_, v interface{}) bool {
		sm, ok := v.(*SockMap)
		if !ok {
			return true
		}
		for _, s := range sm.Sockets() {
			if _, hit := want[s.Acct.ID]; hit {
				s.Enqueue(d)
				sent++
			}
		}
		return true
	})
	return sent
}

// SendToEveryAccount queues a message for every connection held by any signed-in
// account, in a single walk of the directory. It is the fan-out behind a
// broadcast: one message stored one time and delivered to everybody at once
// (arch/NOTIFICATIONS.md).
//
// It skips anonymous sockets. A broadcast lands in the bell, an anonymous
// visitor has no bell, and the frame would be a message that visitor can never
// see or clear. Reaching them is the site notice banner's job.
//
// Note what it cannot carry: a per-account unread count. One frame goes to
// every account and their counts all differ, so the payload carries the message
// alone — see proto.NotifyBroadcastMessage for what the client does with that.
//
// Returns the number of connections the message was queued for.
func SendToEveryAccount(d []byte) int {
	sent := 0
	Map.Range(func(_, v interface{}) bool {
		sm, ok := v.(*SockMap)
		if !ok {
			return true
		}
		for _, s := range sm.Sockets() {
			// 0 is the anonymous marker.
			if s.Acct.ID != 0 {
				s.Enqueue(d)
				sent++
			}
		}
		return true
	})
	return sent
}

// CloseForUID sends a close frame with the given code to every tracked
// connection belonging to one session uid, across every channel, then shuts
// those connections down. It is moderation's socket-level reach
// (arch/ADMIN_MODERATION.md): revoking a banned account's session rows stops
// them making new *requests*, but an already-open WebSocket keeps working —
// it authenticated once at upgrade time and is keyed by uid thereafter. Without
// this, a banned player could keep playing the game they are sitting in.
//
// Returns the number of connections closed.
func CloseForUID(uid string, code int, reason string) int {
	if uid == "" {
		return 0
	}
	closed := 0
	Map.Range(func(_, v interface{}) bool {
		sm, ok := v.(*SockMap)
		if !ok {
			return true
		}
		for _, s := range sm.SocketsFor(uid) {
			if s.Connection != nil {
				_ = s.Connection.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(code, reason),
					time.Now().Add(WriteWait))
			}
			s.Close()
			closed++
		}
		return true
	})
	return closed
}

// CloseAllOn closes every connection tracked on one channel, with the given
// code. The per-channel counterpart to CloseAll: an operator force-closing a
// single room needs its occupants disconnected, not the whole site's.
//
// Peek rather than GetSockMap, so closing a room that has no tracked sockets
// does not create a map for it on the way out.
func CloseAllOn(channelName string, code int, reason string) int {
	sm := Map.Peek(channelName)
	if sm == nil {
		return 0
	}
	closed := 0
	for _, s := range sm.Sockets() {
		if s.Connection != nil {
			_ = s.Connection.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(code, reason),
				time.Now().Add(WriteWait))
		}
		s.Close()
		closed++
	}
	return closed
}

// CloseAll sends a close frame with the given code to every tracked connection
// on every channel, then shuts each connection's writer down. It is the
// shutdown drain's client notification: code 1012 (Service Restart) surfaces
// as evt.code in the browser's onclose, telling clients this is a deploy —
// reconnect promptly — rather than a network failure. WriteControl is safe
// concurrently with the connection's writer goroutine.
func CloseAll(code int, reason string) {
	Map.Range(func(_, v interface{}) bool {
		sm, ok := v.(*SockMap)
		if !ok {
			return true
		}
		for _, s := range sm.Sockets() {
			if s.Connection != nil {
				_ = s.Connection.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(code, reason),
					time.Now().Add(WriteWait))
			}
			s.Close()
		}
		return true
	})
}
