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

-- name: UsernameTaken :one
-- Signup-form availability probe (also covers the reserved-word check's DB
-- side; the in-code reserved list is checked first).
SELECT EXISTS(SELECT 1 FROM users WHERE lower(username) = lower($1));

-- name: UpdatePasswordHash :exec
-- Password change and rehash-on-login (when stored PHC params lag current).
UPDATE users SET password_hash = $2 WHERE id = $1;
