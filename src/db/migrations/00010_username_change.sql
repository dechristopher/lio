-- +goose Up

-- One-time username change (arch/ACCOUNTS_AUTH_RATINGS.md polish pass): an
-- account may change its username exactly once, and only to alter the
-- capitalization. The lower(username) unique index is unaffected — the
-- lowercased identity is immutable, so a casing change never collides. NULL =
-- the one allowed change has not been used; a non-NULL timestamp records when
-- it was and blocks any further rename.
ALTER TABLE users ADD COLUMN username_changed_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE users DROP COLUMN IF EXISTS username_changed_at;
