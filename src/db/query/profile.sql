-- Public player pages (arch/ADMIN_MODERATION.md). These read by account id —
-- the pre-existing ListPlayerGames is keyed by session *uid*, which identifies
-- a browser session rather than a person, so it cannot answer "this account's
-- history". Both games.white_user_id and games.black_user_id carry partial
-- indexes (00006), which is what keeps the OR scans here cheap.
--
-- ## Deriving a per-game result — read before adding a query here
--
-- Every aggregate below decides the account's result from `outcome` plus which
-- seat it held. There is no per-game score column to read instead:
-- games.white_match_score / black_match_score hold the *cumulative match score*
-- after the game (room.go archives player.Score(), which accumulates across a
-- room's games), which is why 00019 renamed them to say so. They coincide with
-- the game's own result only for game 1 of a match — under the old names,
-- `white_score = 1` silently miscounted every later game, and a 3-game match
-- reaching 1.5 fell out of all three of the =1/=0.5/=0 buckets while the total
-- still counted it.
--
-- outcome is unambiguous ('1-0' / '0-1' / '1/2-1/2') and is what HeadToHead
-- already uses. The seat test is IS NOT DISTINCT FROM, not `=`: a bot or
-- anonymous seat stores NULL, and `NULL = 1` is NULL rather than false.

-- name: ListGamesForUserID :many
-- The account's recent games, newest first, with both seats resolved (username
-- + title code) so the page can render the opponent without a query per row.
-- The caller picks the opponent side by comparing the user id, and derives the
-- account's result from outcome + seat (see the note above) rather than from
-- the cumulative score columns.
SELECT g.game_id, g.room_id, g.game_index, g.start_ts,
       g.variant_name, g.variant_group, g.casual, g.rated,
       g.outcome, g.reason, g.bot_persona,
       g.white_user_id, g.black_user_id, g.white_uid, g.black_uid,
       -- per-seat rating going in and the change the game caused, plus the
       -- game's length. All already on the row, so the list can say what
       -- happened rather than only that something did.
       g.white_rating, g.black_rating,
       g.white_rating_delta, g.black_rating_delta,
       (octet_length(g.moves) / 2)::int AS plies,
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
-- Lifetime record from this account's own perspective.
--
-- first_game / played_seconds ride along rather than taking a query of their
-- own: this scan already visits exactly the right rows, and the identity card
-- wants all three together ("Member since March 2026 · 1,204 games · 31 hours
-- played"). played_seconds sums wall-clock game duration, so it counts thinking
-- time on both sides of the board, not just this account's clock.
WITH mine AS (
    SELECT start_ts, end_ts,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE white_user_id = $1
       OR black_user_id = $1
)
SELECT count(*)                            AS games,
       count(*) FILTER (WHERE score = 1)   AS wins,
       count(*) FILTER (WHERE score = 0.5) AS draws,
       count(*) FILTER (WHERE score = 0)   AS losses,
       min(start_ts)::timestamptz          AS first_game,
       COALESCE(SUM(EXTRACT(EPOCH FROM (end_ts - start_ts))), 0)::bigint
                                           AS played_seconds
FROM mine;

-- name: ProfileByVariant :many
-- The same record split by time control, which is how a player actually thinks
-- about their results. Ordered by volume so the page leads with what they play.
WITH mine AS (
    SELECT variant_name, variant_group,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE white_user_id = $1
       OR black_user_id = $1
)
SELECT variant_name, variant_group,
       count(*)                            AS games,
       count(*) FILTER (WHERE score = 1)   AS wins,
       count(*) FILTER (WHERE score = 0.5) AS draws,
       count(*) FILTER (WHERE score = 0)   AS losses
FROM mine
GROUP BY variant_name, variant_group
ORDER BY count(*) DESC, variant_name;

-- name: ProfileVsBots :many
-- Record against each bot persona. A bot seat is the one with no uid and no
-- account (game.SeatIsBot's rule, expressed in SQL); NULL bot_persona predates
-- the persona ladder and resolves to the full-strength Queen in the display
-- layer, exactly as the archive does.
WITH mine AS (
    SELECT bot_persona,
           (CASE WHEN outcome = '1/2-1/2' THEN 0.5
                 WHEN outcome = (CASE WHEN white_user_id IS NOT DISTINCT FROM $1
                                          THEN '1-0' ELSE '0-1' END) THEN 1
                 ELSE 0 END) AS score
    FROM games
    WHERE (white_user_id = $1 AND black_user_id IS NULL AND black_uid = '')
       OR (black_user_id = $1 AND white_user_id IS NULL AND white_uid = '')
)
SELECT bot_persona,
       count(*)                            AS games,
       count(*) FILTER (WHERE score = 1)   AS wins,
       count(*) FILTER (WHERE score = 0.5) AS draws,
       count(*) FILTER (WHERE score = 0)   AS losses
FROM mine
GROUP BY bot_persona
ORDER BY count(*) DESC;
