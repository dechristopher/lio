package db

import (
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
)

// Broadcasts: one message written once and read by every account
// (arch/NOTIFICATIONS.md).
//
// The shape a broadcast is *not* is one notifications row per account. That
// stores the same sentence once per user, grows the write with the user table,
// and still misses everybody who registers afterwards. So the message is one
// row, and each reader's state is derived: a watermark on the account for an
// ordinary announcement, and a broadcast_acks row for one that demands an
// answer.
//
// The cost of that shape is that a broadcast leaves no per-account history. Once
// it is retired it is gone from every panel, where a notification stays until
// its reader clears it. That is the right trade for a site-wide announcement,
// and it is the reason the operator message composer still exists: something
// one player needs to keep is addressed to them.

// Broadcast is one message as the panel renders it, resolved for one reader.
type Broadcast struct {
	ID      int64
	Created time.Time
	Body    string
	Link    string
	// Choices are the answers the message demands, in the order they are shown,
	// and nil for an ordinary announcement. A message with choices is not
	// finished by being seen — only by being answered.
	Choices []string
	// Response is what this reader chose, empty while the question is
	// outstanding (and always empty on a message that asks none).
	Response string
	// Read is this reader's state, decided in SQL: the watermark for an
	// announcement, the presence of an answer for a question. It is computed
	// there rather than here because the badge's count applies the same rule,
	// and two copies of it would eventually disagree.
	Read bool
	// Expires is when the message stops being shown, zero for one that runs
	// until it is retired.
	Expires time.Time
}

// Answered reports whether this reader has answered a message that asks a
// question. False for a message that asks none.
func (b Broadcast) Answered() bool { return b.Response != "" }

// NewBroadcast is one message to send.
type NewBroadcast struct {
	// ActorID is the admin who sent it, recorded for the audit trail and never
	// rendered to the recipient: a broadcast speaks for the site.
	ActorID *int64
	Body    string
	Link    string
	// Choices makes the message an acknowledgement. Empty for an announcement.
	Choices []string
	// Expires bounds how long it shows. Zero runs until it is retired.
	Expires time.Time
}

// CreateBroadcast sends one message to every account and returns the row it
// wrote, with the timestamp the database assigned — the caller delivers that
// same row to the open sockets, and a client-made timestamp would sort
// differently from the copy the panel loads later.
func CreateBroadcast(n NewBroadcast) (Broadcast, error) {
	if Pool == nil {
		return Broadcast{}, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).CreateBroadcast(ctx, gen.CreateBroadcastParams{
		ActorID:   n.ActorID,
		Body:      n.Body,
		Link:      n.Link,
		Choices:   n.Choices,
		ExpiresAt: pgtype.Timestamptz{Time: n.Expires, Valid: !n.Expires.IsZero()},
	})
	if err != nil {
		return Broadcast{}, err
	}
	// A live message now exists, so the fast path below must stop answering
	// "nothing is broadcast" before the next reader connects.
	invalidateLiveBroadcasts()
	return Broadcast{
		ID:      row.ID,
		Created: row.CreatedAt.Time,
		Body:    n.Body,
		Link:    n.Link,
		Choices: n.Choices,
		Expires: n.Expires,
	}, nil
}

// RetireBroadcast pulls a message: it stops showing, in every panel, from now.
// Reports false when it had already ended.
func RetireBroadcast(id int64) (bool, error) {
	if Pool == nil {
		return false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).RetireBroadcast(ctx, id)
	if err != nil {
		return false, err
	}
	invalidateLiveBroadcasts()
	return n > 0, nil
}

// liveBroadcastTTL bounds how stale the "is anything being broadcast" flag may
// be. The writers invalidate it immediately, so this only covers a message
// reaching its own expires_at — nothing writes at that moment, and a message
// that is a few seconds late to disappear from a badge is not worth a sweeper.
const liveBroadcastTTL = 15 * time.Second

// liveBroadcasts caches how many messages are currently showing, for anybody.
//
// It exists to make the common case free. Every socket connect of a signed-in
// account asks for that account's unread count, and adding a second query to
// that path for a table that is almost always empty would be a permanent cost
// for a rare feature. While this is zero, no connect and no panel open touches
// the broadcast tables at all.
//
// It is a count of live rows, not of anybody's unread — it says nothing about
// any one reader, so one process-wide number is a correct thing to share.
var liveBroadcasts = struct {
	sync.Mutex
	n       int64
	fetched time.Time
}{}

func invalidateLiveBroadcasts() {
	liveBroadcasts.Lock()
	liveBroadcasts.fetched = time.Time{}
	liveBroadcasts.Unlock()
}

// AnyLiveBroadcast reports whether any message is currently being broadcast. It
// is the guard every per-reader broadcast read sits behind.
func AnyLiveBroadcast() bool {
	if Pool == nil {
		return false
	}
	liveBroadcasts.Lock()
	defer liveBroadcasts.Unlock()
	if time.Since(liveBroadcasts.fetched) < liveBroadcastTTL {
		return liveBroadcasts.n > 0
	}
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).CountLiveBroadcasts(ctx)
	if err != nil {
		// Answer from the last known state rather than claiming the site has
		// nothing to say. A failed probe must not silently hide an announcement.
		return liveBroadcasts.n > 0
	}
	liveBroadcasts.n = n
	liveBroadcasts.fetched = time.Now()
	return n > 0
}

// UnreadBroadcasts counts the messages one account has neither read nor
// answered. It is added to the account's unread notifications to make one badge.
func UnreadBroadcasts(userID int64) int64 {
	if Pool == nil || userID == 0 || !AnyLiveBroadcast() {
		return 0
	}
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).CountUnreadBroadcasts(ctx, userID)
	if err != nil {
		// Show no badge rather than fail the render, exactly as the
		// notifications count does. The next socket connect asks again.
		return 0
	}
	return n
}

// BroadcastsFor returns the live messages one account should see, newest first,
// each carrying that account's own answer and read state.
func BroadcastsFor(userID int64, limit int32) ([]Broadcast, error) {
	if Pool == nil || userID == 0 || !AnyLiveBroadcast() {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListBroadcastsFor(ctx, gen.ListBroadcastsForParams{
		ID: userID, Limit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Broadcast, 0, len(rows))
	for _, r := range rows {
		out = append(out, Broadcast{
			ID:       r.ID,
			Created:  r.CreatedAt.Time,
			Body:     r.Body,
			Link:     r.Link,
			Choices:  r.Choices,
			Response: strOrEmpty(r.Response),
			Read:     r.IsRead,
			Expires:  r.ExpiresAt.Time,
		})
	}
	return out, nil
}

// MarkBroadcastsSeen moves one account's watermark to now: every announcement
// sent up to this moment has been seen.
//
// It leaves an unanswered question outstanding, and needs no clause to do so —
// those are read through broadcast_acks, which this never writes.
func MarkBroadcastsSeen(userID int64) error {
	if Pool == nil || userID == 0 || !AnyLiveBroadcast() {
		return nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).MarkBroadcastsSeen(ctx, userID)
}

// SeeBroadcast moves one account's watermark up to one message: that message,
// and everything older, has been seen. The move is monotonic, so reading an old
// row after a new one cannot un-read the new one.
func SeeBroadcast(userID, id int64) error {
	if Pool == nil || userID == 0 {
		return nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).AdvanceBroadcastWatermark(ctx,
		gen.AdvanceBroadcastWatermarkParams{ID: userID, ID_2: id})
}

// AnswerBroadcast records one account's answer to a message that demands one.
// Reports false when nothing was written: the choice is not one the message
// offered, the message has ended, or this account already answered it. All
// three are the same answer to the caller — the question is not open to them —
// and distinguishing them would report the state of a message to somebody it is
// not addressed to.
func AnswerBroadcast(id, userID int64, choice string) (bool, error) {
	if Pool == nil || userID == 0 {
		return false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).AckBroadcast(ctx, gen.AckBroadcastParams{
		BroadcastID: id, UserID: userID, Choice: choice,
	})
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// BroadcastRecord is one sent message as the operator sees it on /system:
// what was said, whether it is still showing, and how it was answered.
type BroadcastRecord struct {
	ID      int64
	Created time.Time
	Body    string
	Link    string
	Choices []string
	Expires time.Time
	// Actor is the admin who sent it, empty after that account is deleted.
	Actor string
	// Answers is how many accounts replied, and Tally the breakdown by choice,
	// most-chosen first. Both are zero and nil on an announcement.
	Answers int64
	Tally   []BroadcastAnswer
}

// Live reports whether the message is still showing.
func (b BroadcastRecord) Live() bool {
	return b.Expires.IsZero() || b.Expires.After(time.Now())
}

// BroadcastAnswer is one option and how many accounts chose it.
type BroadcastAnswer struct {
	Choice string
	Count  int64
}

// ListBroadcasts returns what has been sent, newest first, including messages
// that have already ended — with each one's answers folded in.
//
// Two queries, not one per row: the tally for the whole page is a single
// grouped read keyed by the ids just listed.
func ListBroadcasts(limit int32) ([]BroadcastRecord, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListBroadcasts(ctx, limit)
	if err != nil {
		return nil, err
	}

	out := make([]BroadcastRecord, 0, len(rows))
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		out = append(out, BroadcastRecord{
			ID:      r.ID,
			Created: r.CreatedAt.Time,
			Body:    r.Body,
			Link:    r.Link,
			Choices: r.Choices,
			Expires: r.ExpiresAt.Time,
			Actor:   strOrEmpty(r.ActorUsername),
			Answers: r.Answers,
		})
		if r.Answers > 0 {
			ids = append(ids, r.ID)
		}
	}
	// Nothing has been answered: the grouped read would return no rows, so it is
	// not worth making.
	if len(ids) == 0 {
		return out, nil
	}

	tallies, err := gen.New(Pool).BroadcastTallies(ctx, ids)
	if err != nil {
		// The messages are the point of the page; a missing breakdown is a
		// degraded row rather than a failed one.
		return out, nil
	}
	byID := make(map[int64][]BroadcastAnswer, len(ids))
	for _, t := range tallies {
		byID[t.BroadcastID] = append(byID[t.BroadcastID],
			BroadcastAnswer{Choice: t.Choice, Count: t.Answers})
	}
	for i := range out {
		out[i].Tally = byID[out[i].ID]
	}
	return out, nil
}
