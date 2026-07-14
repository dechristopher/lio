-- +goose Up
-- pgn_object_key uniquely identifies a game's archived PGN and is the archive
-- backfill's idempotency + dedup key: every live-archived row already carries
-- it, so a UNIQUE constraint lets backfill INSERT ... ON CONFLICT DO NOTHING to
-- skip games already recorded and to be safely re-runnable.
CREATE UNIQUE INDEX games_pgn_object_key_key ON games (pgn_object_key);

-- +goose Down
DROP INDEX IF EXISTS games_pgn_object_key_key;
