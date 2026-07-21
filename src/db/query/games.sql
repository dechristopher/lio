-- name: InsertGame :one
INSERT INTO games (
    game_id, start_ts, end_ts, race_to, white_score, black_score, method,
    casual, room_id, creator_uid, white_uid, black_uid, variant_name,
    variant_group, outcome, reason, starting_ofen, moves, pgn_object_key,
    game_index, white_user_id, black_user_id, creator_user_id, rated,
    white_rating, black_rating, white_rating_delta, black_rating_delta,
    bot_persona
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
    $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29
)
RETURNING id;

-- name: InsertGameIfNew :one
-- Same columns/order as InsertGame (so the generated param structs are
-- identical and convert directly) but a no-op on a pgn_object_key conflict —
-- the archive backfill's idempotency + dedup-against-live path.
INSERT INTO games (
    game_id, start_ts, end_ts, race_to, white_score, black_score, method,
    casual, room_id, creator_uid, white_uid, black_uid, variant_name,
    variant_group, outcome, reason, starting_ofen, moves, pgn_object_key,
    game_index, white_user_id, black_user_id, creator_user_id, rated,
    white_rating, black_rating, white_rating_delta, black_rating_delta,
    bot_persona
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17,
    $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29
)
ON CONFLICT (pgn_object_key) DO NOTHING
RETURNING id;

-- name: NextGameIndex :one
-- The next 1-based ordinal for a room's game, derived inside the archive
-- transaction (restart-proof; the partial unique games_room_game_idx backstops
-- the practically-impossible concurrent-archive race).
SELECT (COALESCE(MAX(game_index), 0) + 1)::smallint FROM games
WHERE room_id = $1;

-- name: ListRoomGames :many
SELECT * FROM games WHERE room_id = $1 ORDER BY game_index;

-- name: GetRoomGameByIndex :one
SELECT * FROM games WHERE room_id = $1 AND game_index = $2;

-- name: CountGames :one
SELECT count(*) FROM games;

-- name: GetGameByUUID :one
SELECT * FROM games WHERE game_id = $1;

-- name: ListPlayerGames :many
SELECT * FROM games
WHERE white_uid = $1 OR black_uid = $1
ORDER BY start_ts DESC
LIMIT $2 OFFSET $3;

-- name: HeadToHead :one
-- All-time head-to-head between two accounts: each side's cumulative score
-- (win = 1, draw = ½) across every archived game they played against each
-- other, plus the total game count. Symmetric in the two args (@user_a /
-- @user_b). Powers the historical rivalry score beside the match timeline.
SELECT
    COALESCE(SUM(CASE
        WHEN white_user_id = @user_a AND outcome = '1-0' THEN 1.0
        WHEN black_user_id = @user_a AND outcome = '0-1' THEN 1.0
        WHEN outcome = '1/2-1/2' THEN 0.5
        ELSE 0.0 END), 0)::double precision AS a_score,
    COALESCE(SUM(CASE
        WHEN white_user_id = @user_b AND outcome = '1-0' THEN 1.0
        WHEN black_user_id = @user_b AND outcome = '0-1' THEN 1.0
        WHEN outcome = '1/2-1/2' THEN 0.5
        ELSE 0.0 END), 0)::double precision AS b_score,
    COUNT(*)::bigint AS games
FROM games
WHERE (white_user_id = @user_a AND black_user_id = @user_b)
   OR (white_user_id = @user_b AND black_user_id = @user_a);
