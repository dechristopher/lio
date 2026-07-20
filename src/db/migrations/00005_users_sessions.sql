-- +goose Up

-- users: one row per registered account. username preserves display case; the
-- lower(username) unique index enforces case-insensitive uniqueness. email is
-- optional (no email infrastructure yet — recovery is unavailable without it,
-- warned at signup). password_hash is a PHC-format Argon2id string whose
-- embedded parameters give algorithm agility (rehash-on-login when the current
-- params differ). MFA columns arrive with a later migration.
CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    username      TEXT        NOT NULL,
    email         TEXT,
    password_hash TEXT        NOT NULL
);

CREATE UNIQUE INDEX users_username_lower_key ON users (lower(username));

-- sessions: the unified identity system — every visitor, anonymous or
-- authenticated, carries exactly one session (the sid cookie's opaque 256-bit
-- token; only its SHA-256 lands here). uid is the per-session seat/socket
-- identity (16-char base58, the same shape the old cookie identity used):
-- rooms, sockets and the games archive key off it. user_id is NULL for
-- anonymous sessions; login upgrades the row in place (token rotated against
-- fixation, uid preserved so a live seat survives mid-game login). Sessions
-- are revocable auth artifacts, never identity history — games associate to
-- users via their own user_id columns, not through this table.
CREATE TABLE sessions (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    user_id    BIGINT      REFERENCES users (id) ON DELETE CASCADE,
    token_hash BYTEA       NOT NULL UNIQUE,
    uid        TEXT        NOT NULL,
    user_agent TEXT        NOT NULL DEFAULT ''
);

-- the "active sessions" listing for a logged-in user; anon rows (the vast
-- majority) stay out of the index entirely.
CREATE INDEX sessions_user_id_idx ON sessions (user_id) WHERE user_id IS NOT NULL;
-- the hourly expiry sweep's scan.
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

-- +goose Down
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
