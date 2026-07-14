-- name: UpsertPosition :one
-- Insert a distinct position (deduped by hash) and return its id, without a
-- write on conflict: the CTE inserts only when new, otherwise the SELECT arm
-- returns the existing id.
WITH ins AS (
    INSERT INTO positions (hash, ofen)
    VALUES ($1, $2)
    ON CONFLICT (hash) DO NOTHING
    RETURNING id
)
SELECT id FROM ins
UNION ALL
SELECT id FROM positions WHERE hash = $1
LIMIT 1;

-- name: GetPositionByHash :one
SELECT * FROM positions WHERE hash = $1;

-- name: ListPositionsNeedingEval :many
SELECT id, ofen FROM positions
WHERE eval_cp IS NULL
ORDER BY id
LIMIT $1;

-- name: SetPositionEval :exec
UPDATE positions
SET eval_cp = $2, eval_depth = $3, best_move = $4, evaluated_at = now()
WHERE id = $1;
