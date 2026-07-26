package db

import (
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/title"
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
	// Title is the account's optional display title (the zero Title when
	// unset), resolved through the titles table and shown to the left of the
	// username wherever the name renders. Carried into the session/Viewer and
	// stamped onto seats at claim time.
	Title title.Title
	// TOTPConfirmed reports whether the account has an active TOTP factor
	// (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4). Read off the user row the login
	// path already fetches, so the MFA decision costs no extra query for the
	// common (password + TOTP) case; passkeys are counted separately.
	TOTPConfirmed bool
	// UsernameChanged reports whether the account has used its one allowed
	// (casing-only) username change (arch polish pass). Drives the Edit Profile
	// UI's availability state; the change itself is enforced atomically in SQL.
	UsernameChanged bool
	// Role is the account's site permission level (arch/ADMIN_MODERATION.md),
	// Player for the vast majority. Carried into the session/Viewer so the
	// render can gate the moderation UI.
	Role role.Role
	// Ban is the account's sanction status, decoded from the ban columns. The
	// login path refuses a banned account; the player page renders a neutral
	// closed state and suppresses its ratings.
	Ban BanState
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
	if isUniqueViolation(err) {
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
	row, err := gen.New(Pool).GetUserByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	if err != nil {
		return UserRecord{}, false, err
	}
	u := row.User
	return UserRecord{
		ID:              u.ID,
		Username:        u.Username,
		Email:           u.Email,
		PasswordHash:    u.PasswordHash,
		CreatedAt:       u.CreatedAt.Time,
		Title:           title.New(row.TitleCode, row.TitleName),
		TOTPConfirmed:   u.TotpConfirmedAt.Valid,
		UsernameChanged: u.UsernameChangedAt.Valid,
		Role:            role.Parse(u.Role),
		Ban:             banFrom(u.BannedUntil, u.BanReason),
	}, true, nil
}

// GetUserByUsername fetches a user by case-insensitive username. Returns
// found=false on a miss.
func GetUserByUsername(username string) (UserRecord, bool, error) {
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).GetUserByUsernameLower(ctx, username)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserRecord{}, false, nil
	}
	if err != nil {
		return UserRecord{}, false, err
	}
	u := row.User
	return UserRecord{
		ID:              u.ID,
		Username:        u.Username,
		Email:           u.Email,
		PasswordHash:    u.PasswordHash,
		CreatedAt:       u.CreatedAt.Time,
		Title:           title.New(row.TitleCode, row.TitleName),
		TOTPConfirmed:   u.TotpConfirmedAt.Valid,
		UsernameChanged: u.UsernameChangedAt.Valid,
		Role:            role.Parse(u.Role),
		Ban:             banFrom(u.BannedUntil, u.BanReason),
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

// UserDisplayForID resolves a (nullable) user id to its display-case username
// and optional title, both zero-valued for a nil id (anon/bot seat), a miss,
// or an unconfigured Postgres. Used by the archive page, which has no live
// player record to read the seat's account fields from.
func UserDisplayForID(id *int64) (username string, t title.Title) {
	if id == nil || Pool == nil {
		return "", title.Title{}
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).GetUserDisplayByID(ctx, *id)
	if err != nil {
		return "", title.Title{}
	}
	return row.Username, title.New(row.TitleCode, row.TitleName)
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

// UpdateEmail sets, replaces, or clears (email == nil) a user's optional email.
func UpdateEmail(id int64, email *string) error {
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).UpdateEmail(ctx, gen.UpdateEmailParams{
		ID:    id,
		Email: email,
	})
}

// UpdateUsernameCasing applies the one-time casing-only username change. It
// returns ok=false (no error) when the change was refused by the SQL guard —
// the account already renamed, or the lowercased identity did not match (a
// non-casing change slipped past the caller). The caller validates the
// casing-only rule first; this is the atomic once-only backstop.
func UpdateUsernameCasing(id int64, username string) (ok bool, err error) {
	ctx, cancel := Ctx()
	defer cancel()
	_, err = gen.New(Pool).UpdateUsernameCasing(ctx, gen.UpdateUsernameCasingParams{
		ID:       id,
		Username: username,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
