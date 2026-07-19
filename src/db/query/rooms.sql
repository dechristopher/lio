-- name: UpsertRoom :exec
-- Called inside every game-archive transaction: inserts the room row on the
-- match's first archived game, then keeps the "as of latest game" fields
-- current on each subsequent one. Insert-only fields (first_game_ts, creator,
-- variant, race_to, casual) keep their original values on conflict.
INSERT INTO rooms (
    room_id, first_game_ts, last_game_ts, race_to, game_count, casual,
    creator_uid, white_uid, black_uid, white_score, black_score,
    variant_name, variant_group
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
)
ON CONFLICT (room_id) DO UPDATE SET
    last_game_ts = EXCLUDED.last_game_ts,
    game_count   = EXCLUDED.game_count,
    white_uid    = EXCLUDED.white_uid,
    black_uid    = EXCLUDED.black_uid,
    white_score  = EXCLUDED.white_score,
    black_score  = EXCLUDED.black_score;

-- name: CloseRoom :exec
-- Cosmetic close marker fired from room teardown; a no-op for rooms that never
-- archived a game (no row) and for already-closed rooms.
UPDATE rooms SET closed_at = now()
WHERE room_id = $1 AND closed_at IS NULL;

-- name: RoomIDExists :one
-- All-time room-ID uniqueness check for the room-creation re-roll loop: a new
-- room may never reuse the ID of any archived room (its permalink is forever).
SELECT EXISTS(SELECT 1 FROM rooms WHERE room_id = $1);

-- name: GetRoom :one
SELECT * FROM rooms WHERE room_id = $1;
