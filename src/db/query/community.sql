-- Home-page community panels: who just joined, and who is on top. Both are
-- read on a page polled every few seconds by every visitor, so both are served
-- from a TTL cache in db/community.go rather than being run per request.
--
-- Both exclude currently-banned accounts. `banned_until` is NULL for the
-- overwhelming majority and 'infinity' for a permanent ban, so the one
-- comparison covers both temporary and permanent (see 00015_moderation.sql);
-- an *expired* ban is not a ban and its holder is listed again.

-- name: ListNewestUsers :many
-- The most recently registered accounts, newest first. The title join matches
-- the rest of the user reads: NULL for the vast majority, who hold no title.
--
-- The id comes back so the caller can intersect these rows with the presence
-- snapshot and mark the arrivals who are online right now
-- (arch/HOME_ACTIVITY_STREAMING.md). It is folded on server-side and never
-- reaches a client, exactly like message.OnlineMember.ID.
SELECT u.id, u.username, u.created_at, t.code AS title_code, t.name AS title_name
FROM users u
         LEFT JOIN titles t ON t.id = u.title_id
WHERE u.banned_until IS NULL
   OR u.banned_until <= now()
ORDER BY u.created_at DESC
LIMIT $1;

-- name: ListTopRated :many
-- The rating leaderboard: one row per account — their single best established
-- rating and the category it was earned in. Without the DISTINCT ON, a player
-- strong in three categories would occupy three of the handful of slots and
-- crowd everyone else off the board.
--
-- `rd <= $1` is the provisional filter (rating.provisionalRD): a brand-new
-- account starts at 1500 with RD 350, so an unfiltered ORDER BY rating DESC
-- ranks players who have never finished a game against players who have. $2 is
-- a floor on games played, for the same reason from the other direction.
SELECT x.username, x.title_code, x.title_name, x.category, x.rating, x.games
FROM (SELECT DISTINCT ON (r.user_id) u.username,
                                     t.code AS title_code,
                                     t.name AS title_name,
                                     r.category,
                                     r.rating,
                                     r.games
      FROM ratings r
               JOIN users u ON u.id = r.user_id
               LEFT JOIN titles t ON t.id = u.title_id
      WHERE r.rd <= $1
        AND r.games >= $2
        AND (u.banned_until IS NULL OR u.banned_until <= now())
      ORDER BY r.user_id, r.rating DESC) x
ORDER BY x.rating DESC
LIMIT $3;
