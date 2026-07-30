package db

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
)

// Per-account notifications: the messages behind the bell in the header
// (arch/NOTIFICATIONS.md).
//
// Different from feedback (feedback.go) and from the site notice in settings.
// Feedback is a message about the site with one site-wide read state. The site
// notice goes to everybody and keeps no state at all. A notification belongs to
// exactly one account and that account reads it.

const (
	// KindModAction is a moderation decision the player must know about.
	KindModAction = "mod_action"
	// KindMilestone is an achievement: a rating record, a game count.
	KindMilestone = "milestone"
	// KindSystem is a message from the operators to one account.
	KindSystem = "system"
	// KindChallenge is an invitation to a game from another player. It is the
	// one kind that expires, and the one the panel offers an action on.
	KindChallenge = "challenge"
)

// NotificationKinds are the accepted kinds. They match the CHECK constraint in
// migrations 00021 and 00022. Exported so a writer validates against the same
// set the database accepts, rather than a second list that can drift from it.
var NotificationKinds = []string{KindModAction, KindMilestone, KindSystem, KindChallenge}

// ValidNotificationKind reports whether k is one of NotificationKinds.
func ValidNotificationKind(k string) bool {
	for _, v := range NotificationKinds {
		if k == v {
			return true
		}
	}
	return false
}

// Notification is one message as the panel renders it.
type Notification struct {
	ID      int64
	Created time.Time
	Kind    string
	Body    string
	// Link is where the row goes when somebody clicks it. Empty for a message
	// with no destination.
	Link string
	// Actor names the account that caused the message. Empty for a message from
	// the site, and also empty after the actor deletes their account.
	Actor string
	// Read is the zero time while the row is unread.
	Read time.Time
	// Expires bounds how long the message is worth acting on. The zero time
	// means it never stops being worth reading, which is every kind but a
	// challenge.
	Expires time.Time
}

// Unread reports whether the recipient has not read this row yet.
func (n Notification) Unread() bool { return n.Read.IsZero() }

// NewNotification is one message to write. The caller builds body and link, so
// nothing a client sent reaches either column.
type NewNotification struct {
	UserID int64
	Kind   string
	// ActorID is the account that caused the message, or nil for a message from
	// the site itself.
	ActorID *int64
	Body    string
	Link    string
	// Expires bounds how long the message is worth acting on. Leave it zero for
	// a message that does not expire, which is every kind but a challenge.
	Expires time.Time
}

// CreateNotification writes one message and returns the row it wrote. The
// caller sends that row to the account's open sockets, so it needs the id and
// the timestamp the database assigned.
//
// Returns a zero Notification with no error when Postgres is unconfigured
// (PG-less local dev). A site that cannot store notifications must still run.
func CreateNotification(n NewNotification) (Notification, error) {
	if Pool == nil {
		return Notification{}, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).CreateNotification(ctx, gen.CreateNotificationParams{
		UserID:    n.UserID,
		Kind:      n.Kind,
		ActorID:   n.ActorID,
		Body:      n.Body,
		Link:      n.Link,
		ExpiresAt: pgtype.Timestamptz{Time: n.Expires, Valid: !n.Expires.IsZero()},
	})
	if err != nil {
		return Notification{}, err
	}
	return Notification{
		ID:      row.ID,
		Created: row.CreatedAt.Time,
		Kind:    n.Kind,
		Body:    n.Body,
		Link:    n.Link,
		Expires: n.Expires,
	}, nil
}

// UnreadNotifications counts what one account has not read yet.
//
// This runs one time for each socket connect of a signed-in account, and one
// time for each page render that shows the bell. It is a partial index scan
// over one account's backlog. There is no cache: the count belongs to one
// account, so the process-wide cache that UnreadFeedback uses does not apply
// here, and a per-account cache needs eviction that is not worth its cost.
func UnreadNotifications(userID int64) int64 {
	if Pool == nil {
		return 0
	}
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).CountUnreadNotifications(ctx, userID)
	if err != nil {
		// Show no badge rather than fail the render. The next socket connect
		// asks again.
		return 0
	}
	return n
}

// ListNotifications returns one account's messages, newest first.
func ListNotifications(userID int64, limit int32) ([]Notification, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListNotifications(ctx, gen.ListNotificationsParams{
		UserID: userID,
		Limit:  limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Notification, 0, len(rows))
	for _, r := range rows {
		out = append(out, Notification{
			ID:      r.ID,
			Created: r.CreatedAt.Time,
			Kind:    r.Kind,
			Body:    r.Body,
			Link:    r.Link,
			Actor:   strOrEmpty(r.ActorUsername),
			Read:    r.ReadAt.Time,
			Expires: r.ExpiresAt.Time,
		})
	}
	return out, nil
}

// MarkNotificationRead stamps one row read for one account. ok=false with no
// error means the account does not own that row, or the row was already read.
// Both answers are the same to the caller: nothing changed.
func MarkNotificationRead(id, userID int64) (ok bool, err error) {
	if Pool == nil {
		return false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	_, err = gen.New(Pool).MarkNotificationRead(ctx, gen.MarkNotificationReadParams{
		ID:     id,
		UserID: userID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkAllNotificationsRead clears one account's backlog and returns how many
// rows it changed.
func MarkAllNotificationsRead(userID int64) (int64, error) {
	if Pool == nil {
		return 0, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).MarkAllNotificationsRead(ctx, userID)
}
