-- name: CreateSession :one
-- Mints a session row (anonymous when user_id is NULL — the common case; the
-- middleware mints one per cookie-less visitor on page/API routes).
INSERT INTO sessions (token_hash, uid, user_id, expires_at, user_agent)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: GetSessionByTokenHash :one
-- The per-request identity lookup: session + account (username/title NULL for
-- anon). title is the account's optional display title, carried into the
-- render Viewer so the header (and the viewer's own seat) show it.
SELECT s.id, s.uid, s.user_id, s.expires_at, s.last_seen,
       u.username AS username, u.title AS title
FROM sessions s
LEFT JOIN users u ON u.id = s.user_id
WHERE s.token_hash = $1;

-- name: RotateSessionToken :exec
-- The login upgrade: same row (uid preserved so a live seat survives mid-game
-- login), new token (session-fixation defense), account attached, authed
-- expiry applied.
UPDATE sessions
SET token_hash = $2, user_id = $3, expires_at = $4, last_seen = now()
WHERE id = $1;

-- name: TouchSession :exec
-- Throttled sliding-expiry refresh (the resolver only fires this when
-- last_seen is stale, not per request).
UPDATE sessions SET last_seen = now(), expires_at = $2 WHERE id = $1;

-- name: DeleteSessionByTokenHash :exec
-- Logout: hard-delete the row (revocation is immediate modulo the resolver's
-- short cache TTL).
DELETE FROM sessions WHERE token_hash = $1;

-- name: DeleteSessionsForUser :exec
-- "Sign out everywhere".
DELETE FROM sessions WHERE user_id = $1;

-- name: DeleteSessionsForUserExcept :exec
-- Password change: revoke every other session but keep the changer's.
DELETE FROM sessions WHERE user_id = $1 AND id <> $2;

-- name: DeleteSessionByID :exec
-- Profile-popup session revocation; scoped to the owner so a user can only
-- revoke their own sessions.
DELETE FROM sessions WHERE id = $1 AND user_id = $2;

-- name: ListSessionsForUser :many
-- The "active sessions" fragment.
SELECT id, created_at, last_seen, expires_at, user_agent
FROM sessions
WHERE user_id = $1 AND expires_at > now()
ORDER BY last_seen DESC;

-- name: DeleteExpiredSessions :execrows
-- Hourly sweep; bounds table growth from anonymous drive-by sessions.
DELETE FROM sessions WHERE expires_at <= now();
