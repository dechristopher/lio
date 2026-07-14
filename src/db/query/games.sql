-- name: InsertGame :one
INSERT INTO games (
    game_id, start_ts, end_ts, race_to, white_score, black_score, method,
    casual, room_id, creator_uid, white_uid, black_uid, variant_name,
    variant_group, outcome, reason, starting_ofen, moves, pgn_object_key
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
    $18, $19
)
RETURNING id;

-- name: GetGameByUUID :one
SELECT * FROM games WHERE game_id = $1;

-- name: ListPlayerGames :many
SELECT * FROM games
WHERE white_uid = $1 OR black_uid = $1
ORDER BY start_ts DESC
LIMIT $2 OFFSET $3;
