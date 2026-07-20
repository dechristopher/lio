package db

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/dechristopher/lio/db/gen"
)

// This file is the accounts data plane: user rows for the auth package
// (arch/ACCOUNTS_AUTH_RATINGS.md). Unlike the archive accessors these do not
// silently degrade — the auth package checks Ready() first and falls back to
// its own in-memory anonymous store when Postgres is unconfigured, so these
// are only ever called with a live pool.

// ErrUsernameTaken maps the lower(username) unique-index violation so handlers
// can answer "username taken" without leaking Postgres error text.
var ErrUsernameTaken = errors.New("username taken")

// UserRecord is the decoupled user row handed to the auth package.
type UserRecord struct {
	ID           int64
	Username     string
	Email        *string
	PasswordHash string
	CreatedAt    time.Time
}

// CreateUser inserts a registration row, returning the new user's id. A
// violation of the case-insensitive username index returns ErrUsernameTaken.
func CreateUser(username string, email *string, passwordHash string) (int64, error) {
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).CreateUser(ctx, gen.CreateUserParams{
		Username:     username,
		Email:        email,
		PasswordHash: passwordHash,
	})
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return 0, ErrUsernameTaken
	}
	if err != nil {
		return 0, err
	}
	return row.ID, nil
}

// GetUserByID fetches a user by id. Returns found=false on a miss. Used by the
// password-change path to verify the current password.
func GetUserByID(id int64) (UserRecord, bool, error) {
	ctx, cancel := Ctx()
	defer cancel()
	u, err := gen.New(Pool).GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	if err != nil {
		return UserRecord{}, false, err
	}
	return UserRecord{
		ID:           u.ID,
		Username:     u.Username,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt.Time,
	}, true, nil
}

// GetUserByUsername fetches a user by case-insensitive username. Returns
// found=false on a miss.
func GetUserByUsername(username string) (UserRecord, bool, error) {
	ctx, cancel := Ctx()
	defer cancel()
	u, err := gen.New(Pool).GetUserByUsernameLower(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	if err != nil {
		return UserRecord{}, false, err
	}
	return UserRecord{
		ID:           u.ID,
		Username:     u.Username,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt.Time,
	}, true, nil
}

// UsernameTaken reports whether a username is already registered
// (case-insensitive). Used by the signup availability probe.
func UsernameTaken(username string) (bool, error) {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).UsernameTaken(ctx, username)
}

// UsernameForID resolves a (nullable) user id to its display-case username,
// returning "" for a nil id (anon/bot seat) or a miss. Used by the archive
// page to label seats. Degrades to "" when Postgres is unconfigured.
func UsernameForID(id *int64) string {
	if id == nil || Pool == nil {
		return ""
	}
	ctx, cancel := Ctx()
	defer cancel()
	name, err := gen.New(Pool).GetUsernameByID(ctx, *id)
	if err != nil {
		return ""
	}
	return name
}

// UpdatePasswordHash swaps a user's stored PHC string — password changes and
// the login path's rehash-on-login when stored params lag current ones.
func UpdatePasswordHash(id int64, phc string) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).UpdatePasswordHash(ctx, gen.UpdatePasswordHashParams{
		ID:           id,
		PasswordHash: phc,
	})
}
