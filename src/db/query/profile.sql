-- Public player pages (arch/ADMIN_MODERATION.md). These read by account id —
-- the pre-existing ListPlayerGames is keyed by session *uid*, which identifies
-- a browser session rather than a person, so it cannot answer "this account's
-- history". Both games.white_user_id and games.black_user_id carry partial
-- indexes (00006), which is what keeps the OR scans here cheap.

-- name: ListGamesForUserID :many
-- The account's recent games, newest first, with both seats resolved (username
-- + title code) so the page can render the opponent without a query per row.
-- The caller picks the opponent side by comparing the user id.
SELECT g.game_id, g.room_id, g.game_index, g.start_ts,
       g.variant_name, g.variant_group, g.casual, g.rated,
       g.outcome, g.reason, g.bot_persona,
       g.white_user_id, g.black_user_id, g.white_uid, g.black_uid,
       g.white_score, g.black_score,
       wu.username AS white_username, bu.username AS black_username,
       wt.code AS white_title_code, bt.code AS black_title_code
FROM games g
         LEFT JOIN users wu ON wu.id = g.white_user_id
         LEFT JOIN users bu ON bu.id = g.black_user_id
         LEFT JOIN titles wt ON wt.id = wu.title_id
         LEFT JOIN titles bt ON bt.id = bu.title_id
WHERE g.white_user_id = $1
   OR g.black_user_id = $1
ORDER BY g.start_ts DESC
LIMIT $2 OFFSET $3;

-- name: ProfileTotals :one
-- Lifetime record from this account's own perspective. Scores are already
-- per-seat (1 / 0.5 / 0), so the seat CASE is the whole perspective flip.
SELECT count(*)                                                             AS games,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 1)   AS wins,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 0.5) AS draws,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 0)   AS losses
FROM games
WHERE white_user_id = $1
   OR black_user_id = $1;

-- name: ProfileByVariant :many
-- The same record split by time control, which is how a player actually thinks
-- about their results. Ordered by volume so the page leads with what they play.
SELECT variant_name, variant_group,
       count(*)                                                             AS games,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 1)   AS wins,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 0.5) AS draws,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 0)   AS losses
FROM games
WHERE white_user_id = $1
   OR black_user_id = $1
GROUP BY variant_name, variant_group
ORDER BY count(*) DESC, variant_name;

-- name: ProfileVsBots :many
-- Record against each bot persona. A bot seat is the one with no uid and no
-- account (game.SeatIsBot's rule, expressed in SQL); NULL bot_persona predates
-- the persona ladder and resolves to the full-strength Queen in the display
-- layer, exactly as the archive does.
SELECT bot_persona,
       count(*)                                                             AS games,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 1)   AS wins,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 0.5) AS draws,
       count(*) FILTER (WHERE (CASE WHEN white_user_id = $1
                                        THEN white_score ELSE black_score END) = 0)   AS losses
FROM games
WHERE (white_user_id = $1 AND black_user_id IS NULL AND black_uid = '')
   OR (black_user_id = $1 AND white_user_id IS NULL AND white_uid = '')
GROUP BY bot_persona
ORDER BY count(*) DESC;
