-- +goose Up

-- Runtime site controls (arch/ADMIN_MODERATION.md Phase 3): the switches that
-- previously required a deploy to change — the site notice banner, whether
-- registration is open, whether new games are rated, and maintenance mode.
--
-- A key/value table rather than a one-row typed table: the set of switches will
-- keep growing, and a new one should cost a constant in Go and nothing in SQL.
-- Values are TEXT and interpreted by the settings package ("1"/"0" for flags),
-- which owns the defaults — an absent row is the default, so this table starts
-- empty and only records what an admin has actually changed.
--
-- Everything here is deliberately *operational*, never game state: nothing in
-- this table can change the outcome of a game, only whether new ones can start.
CREATE TABLE settings (
    key        TEXT        PRIMARY KEY,
    value      TEXT        NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- who last changed it. Nullable because a value could be seeded by an
    -- operator in SQL with no acting account, exactly as the first admin is.
    updated_by BIGINT      REFERENCES users (id)
);

-- +goose Down
DROP TABLE IF EXISTS settings;
