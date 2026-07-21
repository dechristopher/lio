-- +goose Up

-- Optional display title shown to the left of a player's username wherever the
-- name renders (header, live clocks, match timeline, pre-game cards, archive,
-- and the OG challenge card) — lichess-style ("GM"/"WFM"/…). The view
-- highlights it in the current theme's accent color. Arbitrary free-form text
-- set directly in the DB: there is no in-app assignment UI yet, by design, so
-- the display layer renders the column value verbatim. NULL for the vast
-- majority of accounts (rendered as no title).
ALTER TABLE users
    ADD COLUMN title TEXT;

-- +goose Down
ALTER TABLE users
    DROP COLUMN IF EXISTS title;
