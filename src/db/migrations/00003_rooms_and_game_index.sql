-- +goose Up

-- rooms: one row per room (match) that produced at least one finished game.
-- Upserted at every game archive so the row is always current even if the
-- room's close never lands (crash) — the read path decides liveness via the
-- in-memory room registry, never closed_at, which is cosmetic/analytic only.
-- Rooms that never archive a game are intentionally untracked (their URLs keep
-- 404ing after close). games.room_id stays plain TEXT (no FK): backfilled rows
-- carry room_id = '' which a FK cannot represent, and integrity is procedural
-- inside the archive transaction. UNIQUE(room_id) doubles as the all-time
-- room-ID collision backstop now that room URLs are permanent.
CREATE TABLE rooms (
    id            INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    room_id       TEXT        NOT NULL UNIQUE,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    first_game_ts TIMESTAMPTZ NOT NULL,
    last_game_ts  TIMESTAMPTZ NOT NULL,
    closed_at     TIMESTAMPTZ,
    race_to       INT         NOT NULL DEFAULT 0,
    game_count    INT         NOT NULL DEFAULT 0,
    casual        BOOLEAN     NOT NULL,
    creator_uid   TEXT        NOT NULL,
    white_uid     TEXT        NOT NULL,
    black_uid     TEXT        NOT NULL,
    white_score   REAL        NOT NULL,
    black_score   REAL        NOT NULL,
    variant_name  TEXT        NOT NULL,
    variant_group TEXT        NOT NULL
);

-- game_index: the game's 1-based ordinal within its room (match) — the <n> in
-- the /<room_id>/<n> permalink. 0 for rows with no room association (backfill).
ALTER TABLE games ADD COLUMN game_index SMALLINT NOT NULL DEFAULT 0;

-- backfill ordinals for existing live-archived rows, ordered by start then id
UPDATE games g SET game_index = t.rn
FROM (SELECT id,
             row_number() OVER (PARTITION BY room_id ORDER BY start_ts, id) AS rn
      FROM games WHERE room_id <> '') t
WHERE g.id = t.id;

-- permalink key; the leading column doubles as the room_id lookup index
CREATE UNIQUE INDEX games_room_game_idx ON games (room_id, game_index)
    WHERE room_id <> '';

-- synthesize rooms rows from existing games ("as of latest game" fields come
-- from each room's highest-ordinal row); all historical rooms are closed
INSERT INTO rooms (room_id, first_game_ts, last_game_ts, closed_at, race_to,
                   game_count, casual, creator_uid, white_uid, black_uid,
                   white_score, black_score, variant_name, variant_group)
SELECT agg.room_id, agg.first_ts, agg.last_ts, agg.last_ts, last.race_to,
       agg.n, last.casual, last.creator_uid, last.white_uid, last.black_uid,
       last.white_score, last.black_score, last.variant_name, last.variant_group
FROM (SELECT room_id, min(start_ts) AS first_ts, max(end_ts) AS last_ts,
             count(*) AS n, max(game_index) AS max_idx
      FROM games WHERE room_id <> '' GROUP BY room_id) agg
JOIN games last ON last.room_id = agg.room_id AND last.game_index = agg.max_idx;

-- +goose Down
DROP INDEX IF EXISTS games_room_game_idx;
ALTER TABLE games DROP COLUMN IF EXISTS game_index;
DROP TABLE IF EXISTS rooms;
