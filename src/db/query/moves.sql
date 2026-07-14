-- name: InsertMove :exec
INSERT INTO moves (game_ref, position_id, clock_ms, ply, mv)
VALUES ($1, $2, $3, $4, $5);

-- name: ListGameMoves :many
SELECT * FROM moves WHERE game_ref = $1 ORDER BY ply;

-- name: ListGamesReachingPosition :many
SELECT DISTINCT g.* FROM games g
JOIN moves m ON m.game_ref = g.id
WHERE m.position_id = $1
ORDER BY g.start_ts DESC
LIMIT $2;
