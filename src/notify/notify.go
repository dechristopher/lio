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

// KindAnnounce is the render kind of a broadcast row. It picks the glyph and
// the stripe colour in the client, and it is the one kind that is *not* in
// db.NotificationKinds: a broadcast is not a notifications row, so no CHECK
// constraint stands behind this value. It lives here rather than in db for
// exactly that reason.
const KindAnnounce = "announce"

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
	if s == nil || s.Acct.ID == 0 {
		return
	}
	s.Enqueue(proto.NotifyCountMessage(Unread(s.Acct.ID), staffCount(staff)))
}

// Unread is one account's badge: its own unread notifications plus the
// broadcasts it has neither read nor answered.
//
// One number, because the reader has one bell. The two halves are stored
// differently — a row for each message against a watermark over one shared row
// — but that is a fact about the database, not about what the reader is being
// told, and nothing above this line has to know which half a mark is for.
//
// The broadcast half costs nothing while nothing is being broadcast:
// db.UnreadBroadcasts sits behind a cached "is anything live" flag, which is
// the state the site is in almost always. That matters here because this runs
// on every socket connect of every signed-in account.
func Unread(acctID int64) int64 {
	if acctID == 0 {
		return 0
	}
	return db.UnreadNotifications(acctID) + db.UnreadBroadcasts(acctID)
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
	// One probe, and only for the kind that asks the question. The recipient is
	// the reader here, so "does the reader follow the actor" is answerable
	// directly — the panel's list has a page of rows and batches the same
	// question instead.
	follows := func(actorID int64) bool { return db.IsFollowing(n.UserID, actorID) }
	channel.SendToAccount(n.UserID,
		proto.NotifyMessage(Unread(n.UserID), Item(row, follows)))
	return nil
}

// Broadcast records one message for every account and delivers it to every
// signed-in socket on the site.
//
// The row is written first, for the same reason a notification's is: a frame
// that arrived before its row would show a message the panel cannot list.
//
// The frame carries no count. One frame reaches every account and their counts
// all differ — see proto.NotifyBroadcastMessage, which documents why the client
// adds one itself for this case and only this case. Everybody who is offline
// picks the message up from the count at their next socket connect.
func Broadcast(n db.NewBroadcast) (db.Broadcast, error) {
	row, err := db.CreateBroadcast(n)
	if err != nil {
		return db.Broadcast{}, err
	}
	// PG-less local dev stores nothing and has nothing to deliver.
	if row.ID == 0 {
		return row, nil
	}
	channel.SendToEveryAccount(proto.NotifyBroadcastMessage(BroadcastItem(row)))
	return row, nil
}

// BroadcastItem converts a broadcast into the shape the client renders. It is
// the twin of Item, and for the same reason: the panel's list and the frame
// that arrives live must describe one message identically, so both build it
// here.
//
// The two produce the same shape from different sources, which is what lets the
// client hold one row renderer. Broadcast is the flag that keeps them apart
// where it matters — the id spaces are separate, and the writes go to different
// places.
func BroadcastItem(b db.Broadcast) proto.NotifyItem {
	item := proto.NotifyItem{
		ID:        b.ID,
		Kind:      KindAnnounce,
		Body:      b.Body,
		Link:      b.Link,
		Created:   b.Created.UnixMilli(),
		Read:      b.Read,
		Choices:   b.Choices,
		Response:  b.Response,
		Broadcast: true,
	}
	// Zero means "does not expire" on the wire, and must stay 0 rather than
	// become the unix epoch, which the client reads as long past.
	if !b.Expires.IsZero() {
		item.Expires = b.Expires.UnixMilli()
	}
	return item
}

// FollowLookup answers whether the reader of a notification already follows the
// account that caused it. Item calls it for a follow row and for nothing else.
//
// It is a function rather than a flag because the two callers can answer it at
// very different costs: an arriving message probes for its one actor, while the
// panel's list resolves a whole page in one batched read. A nil lookup means
// "do not ask", which is what a caller with no reader in hand passes.
type FollowLookup func(actorID int64) bool

// Item converts a stored notification into the shape the client renders.
//
// It is the *only* mapping between the two, and it exists because there were
// briefly two: this path and the panel's list endpoint each built the item by
// hand, and both forgot the same field. A challenge then arrived with no expiry,
// which the client reads as already expired — so it rendered as an ordinary
// message with no countdown, no Accept, no Decline and no toast. The feature
// looked built and did nothing.
//
// Anything added to NotifyItem belongs here, once. The follow lookup is an
// argument for the same reason: a viewer-relative field that one path resolved
// and the other did not is the same bug in a new place, and a parameter makes
// the compiler ask every caller.
func Item(n db.Notification, follows FollowLookup) proto.NotifyItem {
	item := proto.NotifyItem{
		ID:       n.ID,
		Kind:     n.Kind,
		Body:     n.Body,
		Link:     n.Link,
		Actor:    n.Actor,
		Created:  n.Created.UnixMilli(),
		Read:     !n.Unread(),
		Choices:  n.Choices,
		Response: n.Response,
	}
	// Zero means "does not expire", and must stay 0 rather than become the unix
	// epoch in milliseconds — which the client would compare against now and
	// treat as long past.
	if !n.Expires.IsZero() {
		item.Expires = n.Expires.UnixMilli()
	}
	// The follow row is the one kind that carries a control about the person who
	// caused it, so it is the one kind that needs the reader's own edge. An actor
	// who deleted their account leaves no id, and there is nobody left to follow.
	if n.Kind == db.KindFollow && n.ActorID != 0 && follows != nil {
		item.Follows = follows(n.ActorID)
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
		proto.NotifyCountMessage(Unread(acctID), staffCount(staff)))
}
