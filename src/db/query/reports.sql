-- Player reports (arch/ADMIN_MODERATION.md Phase 4).

-- name: CreateReport :one
-- File a report. A duplicate open report for the same reporter→target pair
-- violates reports_open_unique; the handler maps that to "already reported"
-- rather than surfacing a constraint error.
INSERT INTO reports (reporter_user_id, target_user_id, game_id, category, note)
VALUES ($1, $2, $3, $4, $5)
RETURNING id;

-- name: ListOpenReports :many
-- The queue, oldest first — it is worked in the order things were reported, so
-- nothing sits at the bottom forever. Both parties' names are resolved here
-- because a queue of user ids is unreadable.
SELECT r.id, r.created_at, r.category, r.note, r.game_id,
       rep.username AS reporter_username,
       tgt.username AS target_username,
       tgt.id       AS target_user_id
FROM reports r
         JOIN users rep ON rep.id = r.reporter_user_id
         JOIN users tgt ON tgt.id = r.target_user_id
WHERE r.status = 'open'
ORDER BY r.created_at
LIMIT $1 OFFSET $2;

-- name: CountOpenReports :one
SELECT count(*) FROM reports WHERE status = 'open';

-- name: ListClosedReports :many
-- Recently resolved, newest first: the queue's own history, so a moderator can
-- see what was already decided about an account before acting again.
SELECT r.id, r.created_at, r.category, r.note, r.game_id,
       r.resolved_at, r.resolution,
       rep.username AS reporter_username,
       tgt.username AS target_username,
       res.username AS resolver_username
FROM reports r
         JOIN users rep ON rep.id = r.reporter_user_id
         JOIN users tgt ON tgt.id = r.target_user_id
         LEFT JOIN users res ON res.id = r.resolved_by
WHERE r.status = 'closed'
ORDER BY r.resolved_at DESC NULLS LAST
LIMIT $1;

-- name: ResolveReport :one
-- Close one open report. Scoped to status = 'open' so two moderators working
-- the queue at once cannot both claim the same row: the second gets no row and
-- is told it was already handled.
UPDATE reports
SET status      = 'closed',
    resolved_at = now(),
    resolved_by = $2,
    resolution  = $3
WHERE id = $1
  AND status = 'open'
RETURNING target_user_id;

-- name: CountOpenReportsForUser :one
-- "Is anyone complaining about this account?" — shown on their player page so a
-- moderator sees the context before acting.
SELECT count(*) FROM reports WHERE target_user_id = $1 AND status = 'open';
