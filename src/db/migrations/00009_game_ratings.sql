-- +goose Up

-- Per-game rating history (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5): the archive
-- view shows each player's rating "at the time of the game" plus the +/- change
-- that game caused. Ratings aren't reconstructable after the fact (only the
-- current rating lives in `ratings`), so each rated game snapshots them here.
--   white_rating / black_rating       : the seat's display rating GOING INTO the
--                                        game ("1650" / "1500?") — what the
--                                        clocks showed during play.
--   white_rating_delta / black_rating_delta : the signed change that game
--                                        applied (+8 / -8).
-- All NULL for casual/anon/bot games and for rows archived before this column
-- existed — the archive then simply shows no rating for that seat.
ALTER TABLE games
    ADD COLUMN white_rating       TEXT,
    ADD COLUMN black_rating       TEXT,
    ADD COLUMN white_rating_delta SMALLINT,
    ADD COLUMN black_rating_delta SMALLINT;

-- +goose Down
ALTER TABLE games
    DROP COLUMN IF EXISTS black_rating_delta,
    DROP COLUMN IF EXISTS white_rating_delta,
    DROP COLUMN IF EXISTS black_rating,
    DROP COLUMN IF EXISTS white_rating;
