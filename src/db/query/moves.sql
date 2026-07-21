-- name: InsertMove :exec
INSERT INTO moves (game_ref, position_id, clock_ms, move_ms, ply, mv)
VALUES ($1, $2, $3, $4, $5, $6);

-- name: ListGameMoves :many
SELECT * FROM moves WHERE game_ref = $1 ORDER BY ply;

-- name: ListGameMoveTimes :many
-- Per-ply timing for one game, in ply order: think time (move_ms) and
-- remaining clock after the move (clock_ms). Both NULL for games archived
-- before timing was recorded.
SELECT ply, clock_ms, move_ms FROM moves WHERE game_ref = $1 ORDER BY ply;

-- name: ListGameMoveEvals :many
-- Cached engine evals for one game's per-ply positions (white-positive
-- centipawns), in ply order. eval_cp is NULL for positions the background
-- evaluator (lio_pg_evaluator) has not reached yet — the archive eval bar
-- renders those plies as unevaluated.
SELECT m.ply, p.eval_cp FROM moves m
JOIN positions p ON p.id = m.position_id
WHERE m.game_ref = $1
ORDER BY m.ply;

-- name: ListGamesReachingPosition :many
SELECT DISTINCT g.* FROM games g
JOIN moves m ON m.game_ref = g.id
WHERE m.position_id = $1
ORDER BY g.start_ts DESC
LIMIT $2;
