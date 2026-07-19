-- +goose Up

-- move_ms: the mover's think time for the ply in milliseconds, as charged by
-- the game clock (net of lag compensation; zero for the uncharged first move).
-- Sibling to clock_ms, which holds the mover's remaining clock after the move
-- (post-increment, the PGN %clk value). Both are NULL for games archived
-- before per-move timing was recorded (including the PGN backfill — old PGNs
-- carry no timing to recover).
ALTER TABLE moves ADD COLUMN move_ms INT;

-- +goose Down
ALTER TABLE moves DROP COLUMN IF EXISTS move_ms;
