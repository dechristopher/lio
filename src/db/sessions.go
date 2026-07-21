package db

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
)

// Session rows for the unified identity system (arch/ACCOUNTS_AUTH_RATINGS.md):
// one row per visitor session, anonymous (user_id NULL) or authenticated. As
// with users.go, the auth package guards Ready() and falls back to an
// in-memory store when Postgres is unconfigured.

// SessionRecord is the decoupled session row (joined with its user, when any)
// handed to the auth package.
type SessionRecord struct {
	ID        int64
	UID       string
	UserID    *int64
	Username  string // empty for anonymous sessions
	Title     string // account's optional display title, empty for anon
	LastSeen  time.Time
	ExpiresAt time.Time
}

// CreateSession mints a session row and returns its id.
func CreateSession(tokenHash []byte, uid string, userID *int64,
	expiresAt time.Time, userAgent string) (int64, error) {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).CreateSession(ctx, gen.CreateSessionParams{
		TokenHash: tokenHash,
		Uid:       uid,
		UserID:    userID,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
		UserAgent: userAgent,
	})
}

// GetSessionByTokenHash resolves a presented token's hash to its session and
// account. Returns found=false on a miss.
func GetSessionByTokenHash(tokenHash []byte) (SessionRecord, bool, error) {
	ctx, cancel := Ctx()
	defer cancel()
	s, err := gen.New(Pool).GetSessionByTokenHash(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return SessionRecord{}, false, nil
	}
	if err != nil {
		return SessionRecord{}, false, err
	}
	rec := SessionRecord{
		ID:        s.ID,
		UID:       s.Uid,
		UserID:    s.UserID,
		Title:     strOrEmpty(s.Title),
		LastSeen:  s.LastSeen.Time,
		ExpiresAt: s.ExpiresAt.Time,
	}
	if s.Username != nil {
		rec.Username = *s.Username
	}
	return rec, true, nil
}

// RotateSessionToken performs the login upgrade in place: new token hash,
// account attached, authenticated expiry applied. The row (and its uid) is
// preserved so a live game seat survives a mid-game login.
func RotateSessionToken(id int64, newHash []byte, userID int64,
	expiresAt time.Time) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).RotateSessionToken(ctx, gen.RotateSessionTokenParams{
		ID:        id,
		TokenHash: newHash,
		UserID:    &userID,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

// TouchSession refreshes last_seen and slides the expiry. The auth resolver
// throttles these (only when last_seen is stale), so this is not per-request.
func TouchSession(id int64, expiresAt time.Time) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).TouchSession(ctx, gen.TouchSessionParams{
		ID:        id,
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
}

// DeleteSessionByTokenHash revokes a single session (logout).
func DeleteSessionByTokenHash(tokenHash []byte) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteSessionByTokenHash(ctx, tokenHash)
}

// DeleteSessionsForUser revokes every session a user holds ("sign out
// everywhere").
func DeleteSessionsForUser(userID int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteSessionsForUser(ctx, &userID)
}

// DeleteSessionsForUserExcept revokes all of a user's sessions but the given
// one — the password-change sweep that keeps the changer logged in.
func DeleteSessionsForUserExcept(userID, keepID int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteSessionsForUserExcept(ctx,
		gen.DeleteSessionsForUserExceptParams{UserID: &userID, ID: keepID})
}

// DeleteSessionByID revokes one of a user's sessions by row id, scoped to the
// owner (the profile popup's per-session revoke).
func DeleteSessionByID(id, userID int64) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteSessionByID(ctx,
		gen.DeleteSessionByIDParams{ID: id, UserID: &userID})
}

// ActiveSession is one row of the profile popup's session list.
type ActiveSession struct {
	ID        int64
	CreatedAt time.Time
	LastSeen  time.Time
	UserAgent string
}

// ListSessionsForUser returns a user's live sessions, most recently seen
// first.
func ListSessionsForUser(userID int64) ([]ActiveSession, error) {
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListSessionsForUser(ctx, &userID)
	if err != nil {
		return nil, err
	}
	out := make([]ActiveSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, ActiveSession{
			ID:        r.ID,
			CreatedAt: r.CreatedAt.Time,
			LastSeen:  r.LastSeen.Time,
			UserAgent: r.UserAgent,
		})
	}
	return out, nil
}

// DeleteExpiredSessions removes every expired session row; returns the count.
// Fired by the auth package's hourly sweep.
func DeleteExpiredSessions() (int64, error) {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).DeleteExpiredSessions(ctx)
}
