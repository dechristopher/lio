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
	// KindFollow is a new follower (arch/FOLLOWING.md). Durable like a
	// moderation decision: no expiry, no action, it simply waits to be read.
	KindFollow = "follow"
)

// NotificationKinds are the accepted kinds. They match the CHECK constraint in
// migrations 00021, 00022 and 00024. Exported so a writer validates against the
// same set the database accepts, rather than a second list that can drift from
// it.
var NotificationKinds = []string{
	KindModAction, KindMilestone, KindSystem, KindChallenge, KindFollow,
}

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
	// ActorID identifies the same account, and is 0 whenever Actor is empty. It
	// is here because a row can carry a viewer-relative control — the follow
	// row's follow-back — and that state is keyed by id, not by a name.
	ActorID int64
	// Read is the zero time while the row is unread.
	Read time.Time
	// Expires bounds how long the message is worth acting on. The zero time
	// means it never stops being worth reading, which is every kind but a
	// challenge.
	Expires time.Time
	// Choices are the answers this message demands, in the order they are shown,
	// and nil for a message that asks nothing. A message with choices is not
	// finished by being seen — only by being answered — which is the general
	// case the live-challenge rule was the first instance of.
	Choices []string
	// Response is what the recipient chose, empty while the question is
	// outstanding.
	Response string
}

// Unread reports whether the recipient has not read this row yet.
func (n Notification) Unread() bool { return n.Read.IsZero() }

// Asks reports whether this message demands an answer before it is finished.
func (n Notification) Asks() bool { return len(n.Choices) > 0 }

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
	// Choices makes the message an acknowledgement: it stays unread, and keeps
	// counting against the badge, until the recipient picks one of them. Leave
	// it empty for a message that only has to be read.
	Choices []string
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
		Choices:   n.Choices,
	})
	if err != nil {
		return Notification{}, err
	}
	out := Notification{
		ID:      row.ID,
		Created: row.CreatedAt.Time,
		Kind:    n.Kind,
		Body:    n.Body,
		Link:    n.Link,
		Expires: n.Expires,
		Choices: n.Choices,
	}
	// The caller gave the actor as a pointer (nil for a message from the site);
	// the row carries it as a plain id, so the delivered item and the same row
	// read back from the panel describe the actor the same way.
	if n.ActorID != nil {
		out.ActorID = *n.ActorID
	}
	return out, nil
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

// RecentFollowNotice reports whether userID has already been told about
// actorID following them within the last day (arch/FOLLOWING.md Phase 3).
//
// It is the suppression on a follow/unfollow/refollow loop, which would
// otherwise announce itself every time round: each new edge really is new, so
// db.Follow reports it as created and the producer would push again.
//
// A failed read answers true — suppress. The cost of being wrong that way is
// one notification somebody does not get; the cost the other way is a panel
// filling with the same name, which is the thing this exists to prevent.
func RecentFollowNotice(userID, actorID int64) bool {
	if Pool == nil || userID == 0 || actorID == 0 {
		return true
	}
	ctx, cancel := Ctx()
	defer cancel()
	found, err := gen.New(Pool).RecentFollowNotice(ctx, gen.RecentFollowNoticeParams{
		UserID: userID, ActorID: &actorID,
	})
	if err != nil {
		return true
	}
	return found
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
		row := Notification{
			ID:       r.ID,
			Created:  r.CreatedAt.Time,
			Kind:     r.Kind,
			Body:     r.Body,
			Link:     r.Link,
			Actor:    strOrEmpty(r.ActorUsername),
			Read:     r.ReadAt.Time,
			Expires:  r.ExpiresAt.Time,
			Choices:  r.Choices,
			Response: strOrEmpty(r.Response),
		}
		if r.ActorID != nil {
			row.ActorID = *r.ActorID
		}
		out = append(out, row)
	}
	return out, nil
}

// AnswerNotification records the recipient's answer to a message that demands
// one, and finishes the row in the same statement.
//
// ok=false with no error covers every way the question is not open to this
// caller: they do not own the row, the row asks nothing, the choice is not one
// it offered, or they already answered. The choice is checked against the row's
// own options inside the query, so a crafted request cannot store an option the
// sender never wrote.
func AnswerNotification(id, userID int64, choice string) (ok bool, err error) {
	if Pool == nil {
		return false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	_, err = gen.New(Pool).AnswerNotification(ctx, gen.AnswerNotificationParams{
		ID:     id,
		UserID: userID,
		// A pointer because response is a nullable column and sqlc types the
		// assignment from it, not from the ARRAY test. The value is never nil:
		// an empty answer is refused above this layer, and would match no row
		// here in any case.
		Choice: &choice,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// MarkNotificationRead stamps one row read for one account. ok=false with no
// error means the account does not own that row, the row was already read, or
// the row asks a question that only an answer can finish. All of those are the
// same answer to the caller: nothing changed.
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

// MarkChallengeReadForRoom retires the challenge notifications one account
// holds for one room, and returns how many rows it changed. link is the value
// the challenge was written with, not a room id — the caller owns that format
// and must build it the same way on both sides.
//
// This is what finishes an accepted invitation. Accepting has no endpoint of
// its own (the panel's Accept is the room link, and the ordinary join follows),
// so the join path calls this. Zero rows is the normal answer for an ordinary
// open room nobody was challenged into.
func MarkChallengeReadForRoom(userID int64, link string) (int64, error) {
	if Pool == nil {
		return 0, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).MarkChallengeNotificationsRead(ctx, gen.MarkChallengeNotificationsReadParams{
		UserID: userID,
		Link:   link,
	})
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
