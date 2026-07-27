package db

import (
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
)

// Player feedback: the "something's wrong / something's great" channel behind
// the prompt in the profile popover, read from the /system console.
//
// Distinct from reports (reports.go) on purpose. A report is an accusation
// about another account and needs a target, a duplicate guard and a decision;
// feedback is a message about the site itself, and the only state it carries is
// whether anyone has read it yet.

// FeedbackKinds are the accepted kinds, matching the CHECK constraint. Exported
// so the handler and the picker validate against exactly the set the database
// will accept rather than a second list that can drift from it.
var FeedbackKinds = []string{"problem", "praise", "idea"}

// ValidFeedbackKind reports whether k is one of FeedbackKinds.
func ValidFeedbackKind(k string) bool {
	for _, v := range FeedbackKinds {
		if k == v {
			return true
		}
	}
	return false
}

// Feedback is one submission as rendered in the inbox.
type Feedback struct {
	ID      int64
	Created time.Time
	Kind    string
	Body    string
	// Path is where the author was when they wrote it — the context that turns
	// "the clock jumps" into something reproducible. Empty when unknown.
	Path   string
	Author string
	// Read is the zero time while nobody has read it; Reader names whoever did.
	Read   time.Time
	Reader string
}

// Unread reports whether this submission is still waiting to be read.
func (f Feedback) Unread() bool { return f.Read.IsZero() }

// SubmitFeedback records one submission.
func SubmitFeedback(userID int64, kind, body, path string) error {
	ctx, cancel := Ctx()
	defer cancel()
	_, err := gen.New(Pool).CreateFeedback(ctx, gen.CreateFeedbackParams{
		UserID: userID,
		Kind:   kind,
		Body:   body,
		Path:   path,
	})
	if err == nil {
		// the new row is unread by definition, so the badge must not keep
		// serving a cached count that predates it
		invalidateUnreadFeedback()
	}
	return err
}

// RecentFeedbackByUser counts what one account has filed inside window, backing
// the per-account submission cap.
func RecentFeedbackByUser(userID int64, window time.Duration) (int64, error) {
	if Pool == nil {
		return 0, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).CountRecentFeedbackByUser(ctx,
		gen.CountRecentFeedbackByUserParams{
			UserID:    userID,
			CreatedAt: pgtype.Timestamptz{Time: time.Now().Add(-window), Valid: true},
		})
}

// ListFeedback returns a page of the inbox, newest first.
func ListFeedback(limit, offset int32) ([]Feedback, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListFeedback(ctx, gen.ListFeedbackParams{
		Limit: limit, Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Feedback, 0, len(rows))
	for _, r := range rows {
		out = append(out, Feedback{
			ID:      r.ID,
			Created: r.CreatedAt.Time,
			Kind:    r.Kind,
			Body:    r.Body,
			Path:    r.Path,
			Author:  r.AuthorUsername,
			Read:    r.ReadAt.Time,
			Reader:  strOrEmpty(r.ReaderUsername),
		})
	}
	return out, nil
}

// CountFeedback returns the total ever submitted, so a bounded inbox page can
// say how much it is not showing.
func CountFeedback() (int64, error) {
	if Pool == nil {
		return 0, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).CountFeedback(ctx)
}

// MarkFeedbackRead stamps one submission as read. ok=false (with no error)
// means it was already read — another moderator got there first, which is
// worth saying rather than silently overwriting their stamp.
func MarkFeedbackRead(id, readerID int64) (ok bool, err error) {
	ctx, cancel := Ctx()
	defer cancel()
	_, err = gen.New(Pool).MarkFeedbackRead(ctx, gen.MarkFeedbackReadParams{
		ID: id, ReadBy: &readerID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	invalidateUnreadFeedback()
	return true, nil
}

// MarkAllFeedbackRead clears the whole backlog, returning how many rows it
// actually flipped.
func MarkAllFeedbackRead(readerID int64) (int64, error) {
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).MarkAllFeedbackRead(ctx, &readerID)
	if err != nil {
		return 0, err
	}
	invalidateUnreadFeedback()
	return n, nil
}

// unreadFeedbackTTL bounds how stale the badge count may be, and doubles as the
// cross-instance propagation delay: an instance that did not serve the
// submission still lights its badge within this window, with no pubsub to
// operate. The local writers invalidate immediately, so a moderator who clears
// the inbox sees the badge go out on their very next request.
const unreadFeedbackTTL = 15 * time.Second

// unreadFeedback caches the badge count. It is read on every page render for a
// moderator (the header dot and the popover's System link), which is far too
// hot for a per-render round trip even though the query itself is a partial
// index scan.
var unreadFeedback = struct {
	sync.Mutex
	n       int64
	fetched time.Time
}{}

// UnreadFeedback returns the number of submissions nobody has read yet,
// refreshing the cached count once it has aged past unreadFeedbackTTL. Returns
// 0 when Postgres is unconfigured (PG-less local dev), which renders no badge —
// the right answer for a site that cannot store feedback in the first place.
func UnreadFeedback() int64 {
	if Pool == nil {
		return 0
	}
	unreadFeedback.Lock()
	defer unreadFeedback.Unlock()
	if time.Since(unreadFeedback.fetched) < unreadFeedbackTTL {
		return unreadFeedback.n
	}
	ctx, cancel := Ctx()
	defer cancel()
	n, err := gen.New(Pool).CountUnreadFeedback(ctx)
	if err != nil {
		// keep serving the previous count rather than blanking the badge: a
		// database blip must not read as "the inbox is empty"
		return unreadFeedback.n
	}
	unreadFeedback.n = n
	unreadFeedback.fetched = time.Now()
	return n
}

// invalidateUnreadFeedback drops the cached count so the next read refetches.
func invalidateUnreadFeedback() {
	unreadFeedback.Lock()
	unreadFeedback.fetched = time.Time{}
	unreadFeedback.Unlock()
}
