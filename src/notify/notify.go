// Package notify delivers per-account notifications to the bell in the header
// (arch/NOTIFICATIONS.md).
//
// It owns two things: the reserved socket channel a page opens when it has no
// other socket, and the write path that records a message and pushes it to the
// account that receives it.
//
// The delivery has no transport of its own. A notification goes out on whatever
// socket the reader already holds — the room socket during a game, the TV
// socket on the home page, or this package's channel everywhere else. The
// sender knows an account, and channel.SendToAccount finds that account's
// connections wherever they are.
//
// Delivery is in-process, which is correct for a deployment of one host (see
// arch/STATE_PERSISTENCE_SCALING.md). It is also not the only path: every
// message is a database row first, and every socket connect reads the count, so
// a frame that never arrives corrects itself the next time the reader opens a
// page.
package notify

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// Channel is the reserved channel key, and the /socket/<chan> path segment, for
// a page that holds no other socket. It is not a room, so the WS handler
// special-cases it exactly as it does the TV channel (see www/ws/ws.go and
// IsNotify).
//
// The name must never collide with a generated room id, which is the same
// constraint the TV channel has.
const Channel = "me"

// IsNotify reports whether the given channel id is the notification channel (as
// opposed to a room id). The WS handler uses it to skip the room-existence
// check.
func IsNotify(id string) bool {
	return id == Channel
}

// Connect sends the unread count to one socket that was just tracked. It is
// called from the WS connection goroutine for every channel, because every
// socket carries notifications.
//
// staff marks a connection that may moderate, which adds the site-wide unread
// feedback count to the same badge. The caller resolves it, because the role is
// on the session it authenticated and this package would have to query for it.
//
// Anonymous sockets get nothing: an anonymous visitor has no name, so nobody
// can address a message to them.
//
// A count of zero is still sent. Zero is the answer that clears a badge the
// page rendered before the reader emptied the panel somewhere else.
func Connect(s *channel.Socket, staff bool) {
	if s == nil || s.AcctID == 0 {
		return
	}
	s.Enqueue(proto.NotifyCountMessage(db.UnreadNotifications(s.AcctID), staffCount(staff)))
}

// staffCount is the unread feedback a moderator sees in the same badge, and 0
// for everybody else. db.UnreadFeedback serves a process-wide cached count, so
// this costs a mutex rather than a query.
func staffCount(staff bool) int64 {
	if !staff {
		return 0
	}
	return db.UnreadFeedback()
}

// Push records one message and delivers it to the recipient's open sockets.
//
// The row is written first. A frame that arrived before its row would show a
// message the panel cannot list, and the badge would disagree with the list the
// moment somebody opened it.
//
// actor is the display name of the account in n.ActorID, for the frame only.
// The row stores the id, so the panel resolves the current name on every read
// and a later rename does not leave an old name behind. The caller has the name
// in hand, which saves this path a second query.
//
// A recipient who is offline is not an error. The row is stored, and the next
// socket connect delivers the count.
func Push(n db.NewNotification, actor string) error {
	if n.UserID == 0 {
		return nil
	}
	if !db.ValidNotificationKind(n.Kind) {
		// A kind the database would reject. Refuse it here rather than let the
		// CHECK constraint report it as a storage failure.
		util.Error(str.CNotif, "refused notification with unknown kind=%s", n.Kind)
		return nil
	}

	row, err := db.CreateNotification(n)
	if err != nil {
		return err
	}
	// PG-less local dev stores nothing and has nothing to deliver.
	if row.ID == 0 {
		return nil
	}

	row.Actor = actor
	channel.SendToAccount(n.UserID,
		proto.NotifyMessage(db.UnreadNotifications(n.UserID), Item(row)))
	return nil
}

// Item converts a stored notification into the shape the client renders.
//
// It is the *only* mapping between the two, and it exists because there were
// briefly two: this path and the panel's list endpoint each built the item by
// hand, and both forgot the same field. A challenge then arrived with no expiry,
// which the client reads as already expired — so it rendered as an ordinary
// message with no countdown, no Accept, no Decline and no toast. The feature
// looked built and did nothing.
//
// Anything added to NotifyItem belongs here, once.
func Item(n db.Notification) proto.NotifyItem {
	item := proto.NotifyItem{
		ID:      n.ID,
		Kind:    n.Kind,
		Body:    n.Body,
		Link:    n.Link,
		Actor:   n.Actor,
		Created: n.Created.UnixMilli(),
		Read:    !n.Unread(),
	}
	// Zero means "does not expire", and must stay 0 rather than become the unix
	// epoch in milliseconds — which the client would compare against now and
	// treat as long past.
	if !n.Expires.IsZero() {
		item.Expires = n.Expires.UnixMilli()
	}
	return item
}

// SendStaffCount pushes the current unread-feedback count to every moderator's
// open sockets. Call it wherever that number changes: a player submits
// feedback, or a moderator reads some.
//
// Without this the staff half of the badge would only ever be correct at the
// moment a socket connects — which is to say, only after a navigation. That was
// the behaviour the old badge poller hid, and dropping the poll for a socket is
// what made an explicit push necessary.
//
// The frame deliberately carries no personal notification count. One frame goes
// to every moderator, and their own counts differ; see proto.NotifyStaffMessage.
func SendStaffCount() {
	ids, err := db.ModeratorIDs()
	if err != nil {
		util.Error(str.CNotif, "staff notify lookup failed error=%s", err.Error())
		return
	}
	if len(ids) == 0 {
		return
	}
	channel.SendToAccounts(ids, proto.NotifyStaffMessage(db.UnreadFeedback()))
}

// SendCount pushes the current unread count to one account's open sockets,
// with no message attached. It is what a reader's *other* tabs need after one
// tab marks something read: each tab holds its own badge, and only the tab that
// made the request learns the new count from the response.
func SendCount(acctID int64, staff bool) {
	if acctID == 0 {
		return
	}
	channel.SendToAccount(acctID,
		proto.NotifyCountMessage(db.UnreadNotifications(acctID), staffCount(staff)))
}
