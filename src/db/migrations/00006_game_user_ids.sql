-- +goose Up

-- Associate archived games and rooms with user accounts
-- (arch/ACCOUNTS_AUTH_RATINGS.md Phase 2). These are the FK columns the
-- 00001 migration anticipated ("they pick up FKs to a users table when
-- accounts land"). All nullable: NULL means an anonymous or bot seat — the
-- pre-existing TEXT uid columns stay as-is for anon/backfill continuity, so no
-- data migration is needed and historical rows are untouched. The rating
-- update (Phase 5) and per-player history read off these ids.
ALTER TABLE games
    ADD COLUMN white_user_id   BIGINT REFERENCES users (id),
    ADD COLUMN black_user_id   BIGINT REFERENCES users (id),
    ADD COLUMN creator_user_id BIGINT REFERENCES users (id);

-- partial: only logged-in seats index (the vast majority of archived games are
-- anon/bot and stay out of it), keeping per-player history scans cheap.
CREATE INDEX games_white_user_id_idx ON games (white_user_id) WHERE white_user_id IS NOT NULL;
CREATE INDEX games_black_user_id_idx ON games (black_user_id) WHERE black_user_id IS NOT NULL;

ALTER TABLE rooms
    ADD COLUMN white_user_id   BIGINT REFERENCES users (id),
    ADD COLUMN black_user_id   BIGINT REFERENCES users (id),
    ADD COLUMN creator_user_id BIGINT REFERENCES users (id);

-- +goose Down
ALTER TABLE rooms
    DROP COLUMN IF EXISTS creator_user_id,
    DROP COLUMN IF EXISTS black_user_id,
    DROP COLUMN IF EXISTS white_user_id;
DROP INDEX IF EXISTS games_black_user_id_idx;
DROP INDEX IF EXISTS games_white_user_id_idx;
ALTER TABLE games
    DROP COLUMN IF EXISTS creator_user_id,
    DROP COLUMN IF EXISTS black_user_id,
    DROP COLUMN IF EXISTS white_user_id;
