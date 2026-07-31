-- The follow graph (arch/FOLLOWING.md). Every query here is keyed by account id
-- on one of the table's two indexes, and none of them reads a column the index
-- does not already carry unless it has to name somebody.
--
-- ## The banned filter — read before adding a query here
--
-- A ban is not a delete. The edge survives it, so an expired ban restores the
-- account to every list and every count it was in before. What a ban does
-- change is visibility: a banned account is excluded from other people's lists
-- AND from their counts, by the same `banned_until` test the home-page panels
-- use (see db/query/community.sql). Both must carry the filter, or a profile
-- shows "12 followers" over a list of 11.
--
-- The one exception is the follow cap (CountFollowing), which counts rows a
-- person owns rather than accounts anybody can see. See its note.

-- name: Follow :execrows
-- Idempotent. The affected-row count is the caller's answer: 1 means a new edge
-- was created, 0 means it already existed. That distinction is what stops an
-- unfollow/refollow loop from being a notification generator — the caller only
-- announces a follow it actually made.
INSERT INTO follows (follower_id, followee_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: Unfollow :execrows
-- Also idempotent: 0 rows means they were not following in the first place,
-- which from the caller's side is the same outcome as success.
DELETE
FROM follows
WHERE follower_id = $1
  AND followee_id = $2;

-- name: IsFollowing :one
-- Does $1 follow $2? One index-only probe of the primary key. Answers the state
-- of the follow button on a player page.
SELECT EXISTS (SELECT 1
               FROM follows
               WHERE follower_id = $1
                 AND followee_id = $2) AS following;

-- name: FollowCounts :one
-- The two numbers on a player page, in one round trip. Each half scans one of
-- the table's two indexes for a single account, which for every real account is
-- a handful of rows.
--
-- No counter columns back these (arch/FOLLOWING.md): a denormalized count costs
-- two extra writes per follow and a repair job the first time a delete path
-- forgets one, to save a scan that is only read on a profile render. If a
-- profile ever gets hot enough to care, the answer is the TTL cache pattern in
-- db/community.go, not a column.
SELECT (SELECT count(*)
        FROM follows f
                 JOIN users u ON u.id = f.followee_id
        WHERE f.follower_id = $1
          AND (u.banned_until IS NULL OR u.banned_until <= now())) AS following,
       (SELECT count(*)
        FROM follows f
                 JOIN users u ON u.id = f.follower_id
        WHERE f.followee_id = $1
          AND (u.banned_until IS NULL OR u.banned_until <= now())) AS followers;

-- name: ListFollowing :many
-- One page of "accounts this player follows", newest follow first.
--
-- The order does not float online members to the top, though the list marks
-- them: this is a directory and its paging must be stable, and a row that
-- jumped pages because somebody opened a tab is worse than a row one line
-- lower. followee_id breaks ties so two follows made in the same instant (a
-- bulk import, a test fixture) cannot swap places between pages.
SELECT u.id, u.username, t.code AS title_code, t.name AS title_name
FROM follows f
         JOIN users u ON u.id = f.followee_id
         LEFT JOIN titles t ON t.id = u.title_id
WHERE f.follower_id = $1
  AND (u.banned_until IS NULL OR u.banned_until <= now())
ORDER BY f.created_at DESC, f.followee_id DESC
LIMIT $2 OFFSET $3;

-- name: ListFollowers :many
-- The mirror: one page of "accounts that follow this player". Same columns in
-- the same order, so one Go type and one client renderer serve both lists.
SELECT u.id, u.username, t.code AS title_code, t.name AS title_name
FROM follows f
         JOIN users u ON u.id = f.follower_id
         LEFT JOIN titles t ON t.id = u.title_id
WHERE f.followee_id = $1
  AND (u.banned_until IS NULL OR u.banned_until <= now())
ORDER BY f.created_at DESC, f.follower_id DESC
LIMIT $2 OFFSET $3;

-- name: FollowedAmong :many
-- Of these accounts, which does $1 follow? An index-only probe of the primary
-- key whose cost is bounded by the size of the array rather than by how many
-- accounts $1 follows (arch/FOLLOWING.md — "The graph answers membership").
--
-- Two callers want exactly this question. A list page asks it about the 25 rows
-- it is about to render, so each row can carry the viewer's own follow state.
-- The online filter asks it about the accounts currently connected. Neither
-- needs a join: the caller already holds the names.
SELECT followee_id
FROM follows
WHERE follower_id = sqlc.arg(follower_id)
  AND followee_id = ANY (sqlc.arg(ids)::bigint[]);

-- name: CountFollowing :one
-- Rows this account owns, for the follow cap. Deliberately unfiltered, unlike
-- the counts above: the cap bounds this account's contribution to the table,
-- and a row pointing at a banned account still occupies the storage the cap
-- exists to bound. It is also an index-only count of the primary key, with no
-- join at all, which is what makes it cheap enough to run before every follow.
SELECT count(*) AS following
FROM follows
WHERE follower_id = $1;
