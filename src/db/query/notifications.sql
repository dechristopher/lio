-- Per-account notifications: the messages behind the bell in the header
-- (arch/NOTIFICATIONS.md).

-- name: CreateNotification :one
-- Returns created_at as well as the id. The caller sends the new row to the
-- account's open sockets, and it must send the timestamp the database wrote
-- rather than one it made itself, so the live row and the same row after a
-- reload sort identically.
INSERT INTO notifications (user_id, kind, actor_id, body, link, expires_at)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING id, created_at;

-- name: CountUnreadNotifications :one
-- The badge. This runs one time for each socket connect of a signed-in account,
-- which is the whole reason the site needs no poll. Served by the partial index.
SELECT count(*) FROM notifications
WHERE user_id = $1 AND read_at IS NULL;

-- name: ListNotifications :many
-- The panel, newest first. Not unread first: a person marks rows read from this
-- list, and an order that moves the rows while they read makes them lose their
-- place. The tint marks the unread rows instead.
--
-- The actor join is LEFT: actor_id is NULL for a message from the site, and it
-- also becomes NULL after an actor deletes their account.
SELECT n.id, n.created_at, n.kind, n.body, n.link, n.read_at, n.expires_at,
       a.username AS actor_username
FROM notifications n
         LEFT JOIN users a ON a.id = n.actor_id
WHERE n.user_id = $1
ORDER BY n.created_at DESC
LIMIT $2;

-- name: MarkNotificationRead :one
-- Scoped by user_id as well as id. The id comes from a client, so the account
-- in the session must own the row. Without the user_id test, any signed-in
-- person could mark somebody else's notification read by guessing an id.
--
-- Also scoped to still-unread, so a second click reports no row instead of
-- moving the timestamp.
UPDATE notifications
SET read_at = now()
WHERE id = $1
  AND user_id = $2
  AND read_at IS NULL
RETURNING id;

-- name: MarkAllNotificationsRead :execrows
-- Clear one account's backlog. Returns the number of rows changed, so the
-- caller reports what happened instead of claiming success over a no-op.
--
-- A challenge that is still live is deliberately left unread. Opening the bell
-- means "I have seen these", which is true of every other kind and finishes
-- them; a challenge is not finished until it is accepted, declined or runs out.
-- Marking it read here would clear the one row that still needs an answer, and
-- the badge would stop saying somebody is waiting on this player.
UPDATE notifications
SET read_at = now()
WHERE user_id = $1
  AND read_at IS NULL
  AND (kind <> 'challenge' OR expires_at IS NULL OR expires_at <= now());
