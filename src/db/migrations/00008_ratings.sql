-- +goose Up

-- Glicko-2 ratings (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5), one row per
-- (user, time-control category). Updated transactionally inside the game
-- archive commit when a rated game between two logged-in players finishes with
-- a real result. A missing row means the player is unrated in that category and
-- defaults to 1500/350/0.06. Rating history is not stored — it is
-- reconstructable from the rated games rows if ever needed.
CREATE TABLE ratings (
    user_id    BIGINT           NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    category   TEXT             NOT NULL,
    rating     DOUBLE PRECISION NOT NULL,
    rd         DOUBLE PRECISION NOT NULL,
    volatility DOUBLE PRECISION NOT NULL,
    games      INTEGER          NOT NULL DEFAULT 0,
    updated_at TIMESTAMPTZ      NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, category)
);

-- rated marks a game/room as ratings-affecting. Additive with a false default,
-- so every existing (casual/anon/bot) row stays unrated with no data migration.
ALTER TABLE games ADD COLUMN rated BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE rooms ADD COLUMN rated BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE rooms DROP COLUMN IF EXISTS rated;
ALTER TABLE games DROP COLUMN IF EXISTS rated;
DROP TABLE IF EXISTS ratings;
