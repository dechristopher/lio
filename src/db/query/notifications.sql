-- Per-account notifications: the messages behind the bell in the header
-- (arch/NOTIFICATIONS.md).

-- name: CreateNotification :one
-- Returns created_at as well as the id. The caller sends the new row to the
-- account's open sockets, and it must send the timestamp the database wrote
-- rather than one it made itself, so the live row and the same row after a
-- reload sort identically.
INSERT INTO notifications (user_id, kind, actor_id, body, link, expires_at, choices)
VALUES ($1, $2, $3, $4, $5, $6, $7)
RETURNING id, created_at;

-- name: CountUnreadNotifications :one
-- The badge. This runs one time for each socket connect of a signed-in account,
-- which is the whole reason the site needs no poll. Served by the partial index.
--
-- An unanswered acknowledgement counts here through read_at, and stays counted:
-- nothing but AnswerNotification can stamp one read (MarkNotificationRead and
-- MarkAllNotificationsRead both skip a row with choices), so the badge keeps
-- reporting a question until it has an answer.
SELECT count(*) FROM notifications
WHERE user_id = $1 AND read_at IS NULL;

-- name: ListNotifications :many
-- The panel, newest first. Not unread first: a person marks rows read from this
-- list, and an order that moves the rows while they read makes them lose their
-- place. The tint marks the unread rows instead.
--
-- The actor join is LEFT: actor_id is NULL for a message from the site, and it
-- also becomes NULL after an actor deletes their account.
--
-- The id comes back beside the name because a row can be viewer-relative: a
-- follow offers a follow-back, which is the question "does the reader already
-- follow this actor" and is answered by id, in one batched probe for the whole
-- page rather than a join per row.
SELECT n.id, n.created_at, n.kind, n.body, n.link, n.read_at, n.expires_at,
       n.choices, n.response, n.actor_id, a.username AS actor_username
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
--
-- A row that asks a question is skipped. Clicking one is not answering it, and
-- stamping it read here would drop it out of the badge while it still needs an
-- answer — AnswerNotification is the only statement that finishes one.
UPDATE notifications
SET read_at = now()
WHERE id = $1
  AND user_id = $2
  AND read_at IS NULL
  AND choices IS NULL
RETURNING id;

-- name: AnswerNotification :one
-- Record the recipient's answer, and finish the row in the same statement: a
-- question that has been answered has certainly been read.
--
-- The choice is validated against the row's own options here rather than in Go.
-- An answer that is not on the list matches nothing and writes nothing, so a
-- crafted request cannot store an option the sender never offered, and no
-- caller can forget the check. Scoped to still-unanswered, so the first answer
-- stands.
UPDATE notifications
SET response = sqlc.arg(choice),
    read_at  = now()
WHERE id = sqlc.arg(id)
  AND user_id = sqlc.arg(user_id)
  AND response IS NULL
  AND choices @> ARRAY [sqlc.arg(choice)::text]
RETURNING id;

-- name: MarkChallengeNotificationsRead :execrows
-- Retire the challenge notifications one account holds for one room, matched on
-- the link the challenge was written with. This is what finishes an accepted
-- invitation: accepting has no endpoint of its own — it is the ordinary join —
-- so the join path calls this instead, and the row stops sitting unread in the
-- panel offering an Accept for a game the person is already playing.
--
-- Scoped to this account's own rows and to the challenge kind, so a link that
-- happens to match some other kind of message is untouched. Plural because a
-- challenger may re-challenge the same person into a new room, and only the
-- rows pointing at the room actually joined are finished.
UPDATE notifications
SET read_at = now()
WHERE user_id = $1
  AND kind = 'challenge'
  AND link = $2
  AND read_at IS NULL;

-- name: MarkAllNotificationsRead :execrows
-- Clear one account's backlog. Returns the number of rows changed, so the
-- caller reports what happened instead of claiming success over a no-op.
--
-- A challenge that is still live is deliberately left unread. Opening the bell
-- means "I have seen these", which is true of every other kind and finishes
-- them; a challenge is not finished until it is accepted, declined or runs out.
-- Marking it read here would clear the one row that still needs an answer, and
-- the badge would stop saying somebody is waiting on this player.
--
-- An unanswered acknowledgement is left for the same reason, and it is the
-- general case the challenge rule was the first instance of: a row that asks a
-- question is not finished by being looked at. Only AnswerNotification finishes
-- one.
UPDATE notifications
SET read_at = now()
WHERE user_id = $1
  AND read_at IS NULL
  AND (choices IS NULL OR response IS NOT NULL)
  AND (kind <> 'challenge' OR expires_at IS NULL OR expires_at <= now());

-- name: RecentFollowNotice :one
-- Has this account already been told about this follower lately?
--
-- Without it, follow → unfollow → follow is a notification generator: each new
-- edge is genuinely new (db.Follow reports it as created), so each one would
-- announce itself. The write path is rate limited and a follow needs an
-- account, but neither of those stops a slow, deliberate loop from filling
-- somebody's panel.
--
-- It rides the existing notifications_recent_idx (user_id, created_at DESC), so
-- it needs no index of its own: the range is one day of one account's messages,
-- and it runs only on a follow that actually created an edge.
SELECT EXISTS (SELECT 1
               FROM notifications
               WHERE user_id = $1
                 AND actor_id = $2
                 AND kind = 'follow'
                 AND created_at > now() - INTERVAL '1 day') AS found;
