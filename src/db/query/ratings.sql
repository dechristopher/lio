-- Glicko-2 ratings data access (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5). The
-- rating update runs inside the archive transaction, so GetRatingForUpdate takes
-- a row lock; the display reads (GetRating / ListRatingsForUser) do not.

-- name: GetRatingForUpdate :one
-- Locks the (user, category) rating row for the transactional update. Callers
-- lock both players' rows in ascending user_id order to avoid deadlocks between
-- two simultaneous games that share a player.
SELECT rating, rd, volatility, games FROM ratings
WHERE user_id = $1 AND category = $2
FOR UPDATE;

-- name: UpsertRating :exec
-- Writes a player's post-game rating, inserting the row on their first rated
-- game in the category.
INSERT INTO ratings (user_id, category, rating, rd, volatility, games, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, now())
ON CONFLICT (user_id, category) DO UPDATE SET
    rating     = EXCLUDED.rating,
    rd         = EXCLUDED.rd,
    volatility = EXCLUDED.volatility,
    games      = EXCLUDED.games,
    updated_at = now();

-- name: GetRating :one
-- Display read for one category (seat-claim rating capture).
SELECT rating, rd, volatility, games FROM ratings
WHERE user_id = $1 AND category = $2;

-- name: ListRatingsForUser :many
-- All of a user's category ratings (profile popover summary).
SELECT category, rating, rd, volatility, games FROM ratings
WHERE user_id = $1
ORDER BY category;
