-- Player feedback: the site-improvement channel behind the profile popover
-- prompt, read from the /system console.

-- name: CreateFeedback :one
INSERT INTO feedback (user_id, kind, body, path)
VALUES ($1, $2, $3, $4)
RETURNING id;

-- name: CountRecentFeedbackByUser :one
-- The per-account submission cap. The caller passes the window's start rather
-- than an interval so the window is a rolling one: a cap that resets at
-- midnight UTC lets someone file twice their budget in one sitting, and the
-- account has no idea when midnight UTC is anyway.
SELECT count(*) FROM feedback
WHERE user_id = $1 AND created_at > $2;

-- name: ListFeedback :many
-- The inbox, newest first. Not unread-first: read state is toggled from this
-- very list, and an ordering that reshuffles the rows underneath the moderator
-- as they work makes them lose their place. Unread rows are tinted instead.
SELECT f.id, f.created_at, f.kind, f.body, f.path, f.read_at,
       u.username  AS author_username,
       rd.username AS reader_username
FROM feedback f
         JOIN users u ON u.id = f.user_id
         LEFT JOIN users rd ON rd.id = f.read_by
ORDER BY f.created_at DESC
LIMIT $1 OFFSET $2;

-- name: CountFeedback :one
SELECT count(*) FROM feedback;

-- name: CountUnreadFeedback :one
-- Drives the red dot in the header and the profile popover. Hit on every
-- moderator page render, so it goes through a short-TTL cache in the db package
-- rather than straight to Postgres.
SELECT count(*) FROM feedback WHERE read_at IS NULL;

-- name: MarkFeedbackRead :one
-- Scoped to still-unread so two moderators clearing the inbox at once cannot
-- overwrite each other's stamp; the second gets no row and is told it was
-- already read.
UPDATE feedback
SET read_at = now(),
    read_by = $2
WHERE id = $1
  AND read_at IS NULL
RETURNING id;

-- name: MarkAllFeedbackRead :execrows
-- Clear the backlog in one go. Returns the number actually flipped so the
-- caller can say what happened rather than claiming success over a no-op.
UPDATE feedback
SET read_at = now(),
    read_by = $1
WHERE read_at IS NULL;
