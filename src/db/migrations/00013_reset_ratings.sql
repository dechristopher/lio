-- +goose Up

-- Ratings are now keyed per exact time control (the variant HTMLName, e.g.
-- "one-two-rapid-deploy") instead of the coarse variant speed group. Every prior
-- row was keyed by the group — and for the only offered mode that group was the
-- single "deploy" bucket, which lumped all four time controls (¼+0, ½+1, 1+2,
-- 3+5) into one rating. That collapsed history cannot be split back apart into
-- the new per-time-control pools, so wipe the table for a clean slate: each
-- account re-provisions from the unrated default (1500?) on its first game per
-- time control.
DELETE FROM ratings;

-- +goose Down

-- Irreversible: the pre-wipe rows cannot be reconstructed.
