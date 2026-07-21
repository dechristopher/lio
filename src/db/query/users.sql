-- name: CreateUser :one
-- Registration insert. Uniqueness rides on the lower(username) unique index;
-- callers map its violation to a "username taken" error.
INSERT INTO users (username, email, password_hash)
VALUES ($1, $2, $3)
RETURNING id, username;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: GetUserByUsernameLower :one
-- Login lookup: case-insensitive, served by the lower(username) unique index.
SELECT * FROM users WHERE lower(username) = lower($1);

-- name: GetUsernameByID :one
-- Resolve a user id to its display-case username (archive page seat labels).
SELECT username FROM users WHERE id = $1;

-- name: GetUserDisplayByID :one
-- Resolve a user id to its display-case username plus optional title, for the
-- archive page's seat labels (which have no live player record to read).
SELECT username, title FROM users WHERE id = $1;

-- name: UsernameTaken :one
-- Signup-form availability probe (also covers the reserved-word check's DB
-- side; the in-code reserved list is checked first).
SELECT EXISTS(SELECT 1 FROM users WHERE lower(username) = lower($1));

-- name: UpdatePasswordHash :exec
-- Password change and rehash-on-login (when stored PHC params lag current).
UPDATE users SET password_hash = $2 WHERE id = $1;

-- name: UpdateEmail :exec
-- Set / replace / clear the account email ($2 NULL clears it). There is no
-- email infrastructure yet, so this is a plain overwrite — no verification.
UPDATE users SET email = $2 WHERE id = $1;

-- name: UpdateUsernameCasing :one
-- The one-time casing-only username change (arch polish pass): rewrite only the
-- display case and stamp username_changed_at, but only when the account has not
-- renamed before (username_changed_at IS NULL) and the lowercased identity is
-- unchanged (casing-only — the caller enforces this too). Returns the new
-- username; no row (pgx.ErrNoRows) means the change was refused (already
-- renamed, or a raced update).
UPDATE users
SET username = $2, username_changed_at = now()
WHERE id = $1
  AND username_changed_at IS NULL
  AND lower(username) = lower($2)
RETURNING username;
