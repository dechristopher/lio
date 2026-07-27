-- Profile statistics reads (arch/PROFILE_STATS.md). Everything here is the same
-- "one account's perspective" shape as profile.sql: the archive stores per-seat
-- values, so each query resolves which seat the account held and reports from
-- that side. They ride the partial white_user_id/black_user_id indexes.

-- name: ProfileRatingHistory :many
-- The account's rating over time, one point per category per day, for the rating
-- curve.
--
-- The point is the rating the account carried INTO that day's first rated game —
-- a value recorded on the row, never a running sum of the per-game deltas. Delta
-- is round(newR - oldR), the rounded *true* change rather than the difference of
-- two rounded values, so accumulating deltas drifts against the stored rating
-- over a long history. Taking the day's first game means every game's effect
-- shows up in the following day's point; the caller closes the series with the
-- account's current rating, so the final day's play is accounted for too.
--
-- DISTINCT ON keeps the earliest game per (category, day) — the ORDER BY must
-- lead with the DISTINCT ON expressions, and start_ts ascending picks "first".
-- The rating string carries its provisional marker ("1500?"), which the display
-- layer reads to render the uncertain prefix differently.
SELECT DISTINCT ON (rating_category, day)
    rating_category::text                                   AS category,
    date_trunc('day', start_ts)::date                       AS day,
    (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
              THEN white_rating ELSE black_rating END)::text AS rating
FROM games
WHERE (white_user_id = $1 OR black_user_id = $1)
  AND rated
  AND rating_category IS NOT NULL
  AND (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                THEN white_rating ELSE black_rating END) IS NOT NULL
ORDER BY rating_category, day, start_ts;

-- name: ProfileColorSplit :many
-- The account's record as White and as Black. In blind-deploy octad the two
-- seats are a genuine strategic asymmetry — White deploys and moves first — so
-- this is a real split rather than the chess platitude it would be elsewhere.
--
-- The seat test is IS NOT DISTINCT FROM, not plain `=`. A bot or anonymous
-- White seat stores NULL in white_user_id, and `NULL = 1` is NULL rather than
-- false — so a plain equality splits the account's Black games into a `false`
-- group and a NULL group, and the NULL then fails to scan into a bool.
--
-- The result comes from outcome, never from the score columns; see the header
-- note in profile.sql for why.
WITH mine AS (
    SELECT (white_user_id IS NOT DISTINCT FROM $1) AS as_white,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE white_user_id = $1
       OR black_user_id = $1
)
SELECT as_white,
       count(*)                            AS games,
       count(*) FILTER (WHERE score = 1)   AS wins,
       count(*) FILTER (WHERE score = 0.5) AS draws,
       count(*) FILTER (WHERE score = 0)   AS losses
FROM mine
GROUP BY as_white;

-- name: ProfileEndings :many
-- How the account's games finish, by the DB-canonical method token
-- (games.reason: checkmate / resignation / time / stalemate / insufficient /
-- repetition / moverule / agreement). Split by result, because "you win by
-- checkmate and lose on time" is the actionable sentence — a bare frequency
-- count is not. Busiest first; the display layer names the tokens.
WITH mine AS (
    SELECT reason,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE white_user_id = $1
       OR black_user_id = $1
)
SELECT reason,
       count(*)                            AS games,
       count(*) FILTER (WHERE score = 1)   AS wins,
       count(*) FILTER (WHERE score = 0.5) AS draws,
       count(*) FILTER (WHERE score = 0)   AS losses
FROM mine
GROUP BY reason
ORDER BY count(*) DESC, reason;

-- name: ProfileLengths :many
-- Game length in plies, with the account's result at each length. moves is the
-- packed move blob — db.BuildPlies writes exactly 2 bytes per ply — so the ply
-- count comes straight off the row with no join to the moves table.
--
-- Grouped by exact ply count rather than a fixed bucket width: octad games are
-- short, so this is a few dozen rows at most, and it leaves the bucketing to the
-- view instead of freezing it into the schema.
WITH mine AS (
    SELECT (octet_length(moves) / 2)::int AS plies,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE (white_user_id = $1 OR black_user_id = $1)
      AND octet_length(moves) > 0
)
SELECT plies,
       count(*)                            AS games,
       count(*) FILTER (WHERE score = 1)   AS wins,
       count(*) FILTER (WHERE score = 0.5) AS draws,
       count(*) FILTER (WHERE score = 0)   AS losses
FROM mine
GROUP BY plies
ORDER BY plies;

-- name: ProfileStreaks :one
-- The account's current result streak and its best-ever winning streak.
--
-- Classic gaps-and-islands: subtracting a per-result row number from the overall
-- row number gives every run of identical results a constant group key, so one
-- GROUP BY collapses each run to its length. (start_ts, id) breaks ties so two
-- games archived in the same instant cannot split a run arbitrarily.
--
-- current_score is only meaningful when current_len > 0; an account with no
-- games yields zeroes, which the display layer reads as "no streak".
WITH results AS (
    SELECT id, start_ts,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE white_user_id = $1
       OR black_user_id = $1
), islands AS (
    SELECT score, count(*) AS len, max(start_ts) AS last_ts
    FROM (
        SELECT score, start_ts,
               row_number() OVER (ORDER BY start_ts, id)
                   - row_number() OVER (PARTITION BY score ORDER BY start_ts, id) AS grp
        FROM results
    ) grouped
    GROUP BY score, grp
)
SELECT
    COALESCE(max(len) FILTER (WHERE score = 1), 0)::int                       AS best_wins,
    COALESCE((SELECT len FROM islands ORDER BY last_ts DESC LIMIT 1), 0)::int AS current_len,
    COALESCE((SELECT score FROM islands ORDER BY last_ts DESC LIMIT 1), 0)::real AS current_score
FROM islands;

-- name: ProfileFormations :many
-- The account's games grouped by the exact deployed position and which seat it
-- held (arch/PROFILE_STATS.md Phase 4).
--
-- The grouping key is the raw starting OFEN because the *pair* of formations is
-- what a starting position is — naming either side's arrangement is the display
-- layer's job (opening.Names), and putting that mapping in SQL would duplicate
-- the one table the opening package exists to own.
--
-- This stays small however much the account has played: octad has 12 formations
-- a side, so at most 144 distinct deploy starts × 2 seats. A player with ten
-- thousand games returns the same couple of hundred rows as one with fifty.
WITH mine AS (
    SELECT starting_ofen,
           (white_user_id IS NOT DISTINCT FROM $1) AS as_white,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE (white_user_id = $1 OR black_user_id = $1)
      AND starting_ofen <> ''
)
SELECT starting_ofen, as_white,
       count(*)                            AS games,
       count(*) FILTER (WHERE score = 1)   AS wins,
       count(*) FILTER (WHERE score = 0.5) AS draws,
       count(*) FILTER (WHERE score = 0)   AS losses
FROM mine
GROUP BY starting_ofen, as_white;

-- name: ProfileActivity :many
-- Games per day over the last year, for the activity heatmap
-- (arch/PROFILE_STATS.md Phase 5).
--
-- Days are bucketed in UTC, deliberately: a public page whose content shifts
-- with the viewer's timezone is one that cannot be cached and cannot be
-- reasoned about, and the alternative — passing a client offset — makes the
-- cache key viewer-dependent for a cosmetic gain at day granularity.
--
-- The window rides games_start_ts_brin: the table is append-only and
-- start_ts-correlated, which is exactly what a block-range index is for.
WITH mine AS (
    SELECT date_trunc('day', start_ts)::date AS day,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE (white_user_id = $1 OR black_user_id = $1)
      AND start_ts >= now() - interval '1 year'
)
SELECT day,
       count(*)                            AS games,
       count(*) FILTER (WHERE score = 1)   AS wins,
       count(*) FILTER (WHERE score = 0.5) AS draws,
       count(*) FILTER (WHERE score = 0)   AS losses
FROM mine
GROUP BY day
ORDER BY day;

-- name: ProfileOpponents :many
-- The accounts this one has played most, with the record against each.
--
-- Only logged-in opponents: an anonymous seat is a browser session rather than
-- a person, so tallying "games against Anonymous" would merge strangers into
-- one fictitious rival. Bot seats are excluded for the same reason — they have
-- their own section, where the persona is what matters.
WITH mine AS (
    SELECT (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                     THEN black_user_id ELSE white_user_id END) AS opponent_id,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE white_user_id = $1
       OR black_user_id = $1
)
-- opponent_id groups but is not selected: the page links by username, and
-- selecting the CASE leaves sqlc unable to infer its type.
SELECT u.username, t.code AS title_code,
       count(*)                              AS games,
       count(*) FILTER (WHERE m.score = 1)   AS wins,
       count(*) FILTER (WHERE m.score = 0.5) AS draws,
       count(*) FILTER (WHERE m.score = 0)   AS losses
FROM mine m
         JOIN users u ON u.id = m.opponent_id
         LEFT JOIN titles t ON t.id = u.title_id
WHERE m.opponent_id IS NOT NULL
GROUP BY m.opponent_id, u.username, t.code
ORDER BY count(*) DESC, u.username
LIMIT $2;
