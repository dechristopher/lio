-- +goose Up

-- Per-account display preferences: the choices a signed-in player makes about
-- what the site shows them, kept with the account rather than with the browser.
--
-- The counterpart to the `settings` table, and deliberately shaped like it: a
-- key/value store whose values are TEXT and whose meaning lives in Go (the
-- prefs package), so a new preference costs a constant and nothing in SQL. An
-- absent row is the default, so this table starts empty and only records what a
-- player has actually changed.
--
-- What belongs here is what must follow the account across devices. Purely
-- local appearance choices — the color theme, the board, the piece set — stay
-- in localStorage, because they have to resolve before first paint and a
-- round trip cannot.
--
-- Nothing here can change the outcome of a game, or what another account sees.
-- A preference decides what its owner is shown, and only that.
CREATE TABLE user_prefs (
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    key        TEXT        NOT NULL,
    value      TEXT        NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, key)
);

-- +goose Down
DROP TABLE IF EXISTS user_prefs;
