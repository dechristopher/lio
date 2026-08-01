-- Broadcasts: one message read by every account (arch/NOTIFICATIONS.md).
--
-- Every read here pairs the message with the reader, because a broadcast holds
-- no per-account state of its own. Two things decide whether one account has
-- finished with one message:
--
--   * the watermark, users.broadcast_seen_at, for an ordinary announcement, and
--   * a row in broadcast_acks, for a message that demands an answer.
--
-- The CASE below encodes exactly that, and it is written once per question
-- rather than assembled in Go, so the count behind the badge and the list
-- behind the panel can never disagree about what "read" means.
--
-- COALESCE(u.broadcast_seen_at, u.created_at) is the watermark's floor: an
-- account that has never read one has still not missed anything sent before it
-- existed. The `b.created_at > u.created_at` test says the same thing about the
-- message, and is what stops a new account opening its bell onto history.

-- name: CreateBroadcast :one
-- Send one. Returns the timestamp the database wrote, because the delivered
-- item and the same message read back from the panel must sort identically.
INSERT INTO broadcasts (actor_id, body, link, choices, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, created_at;

-- name: RetireBroadcast :execrows
-- Pull a message. Retiring is expiring it now: one column answers "when does
-- this stop showing", whether that moment was chosen in advance or forced.
--
-- Scoped to the still-live rows, so retiring twice reports no change instead of
-- moving the timestamp of something that already ended.
UPDATE broadcasts
SET expires_at = now()
WHERE id = $1
  AND (expires_at IS NULL OR expires_at > now());

-- name: CountLiveBroadcasts :one
-- How many messages are currently showing, for any account. It is the whole
-- site's answer, not one reader's, and it backs the cached fast path in
-- db/broadcasts.go: while this is zero, no socket connect and no panel open
-- costs a broadcast query at all. That is the common state of the site.
SELECT count(*)
FROM broadcasts
WHERE expires_at IS NULL
   OR expires_at > now();

-- name: CountUnreadBroadcasts :one
-- The broadcast half of one account's badge, added to the notifications half.
SELECT count(*)
FROM broadcasts b
         CROSS JOIN users u
         LEFT JOIN broadcast_acks a ON a.broadcast_id = b.id AND a.user_id = u.id
WHERE u.id = $1
  AND b.created_at > u.created_at
  AND (b.expires_at IS NULL OR b.expires_at > now())
  AND (CASE
           WHEN b.choices IS NULL
               THEN b.created_at > COALESCE(u.broadcast_seen_at, u.created_at)
           ELSE a.choice IS NULL
    END);

-- name: ListBroadcastsFor :many
-- The live messages for one account's panel, newest first, each with that
-- account's own answer and read state.
--
-- is_read is computed here rather than in Go for the reason given at the top of
-- this file: it is the same rule CountUnreadBroadcasts applies, and two copies
-- of it would eventually disagree — the badge saying one thing and the panel
-- showing another.
SELECT b.id,
       b.created_at,
       b.body,
       b.link,
       b.choices,
       b.expires_at,
       a.choice AS response,
       (CASE
            WHEN b.choices IS NULL
                THEN b.created_at <= COALESCE(u.broadcast_seen_at, u.created_at)
            ELSE a.choice IS NOT NULL
           END)::bool AS is_read
FROM broadcasts b
         CROSS JOIN users u
         LEFT JOIN broadcast_acks a ON a.broadcast_id = b.id AND a.user_id = u.id
WHERE u.id = $1
  AND b.created_at > u.created_at
  AND (b.expires_at IS NULL OR b.expires_at > now())
ORDER BY b.created_at DESC
LIMIT $2;

-- name: MarkBroadcastsSeen :exec
-- Read-all: everything sent up to now has been seen.
--
-- It does not touch an unanswered acknowledgement, and it does not have to:
-- those rows are read through broadcast_acks, which this statement never
-- writes. Seeing a question is not answering it.
UPDATE users
SET broadcast_seen_at = now()
WHERE id = $1;

-- name: AdvanceBroadcastWatermark :exec
-- One row was clicked: everything up to and including that message has been
-- seen. GREATEST keeps the watermark monotonic, so reading an older row after
-- a newer one cannot un-read the newer one.
UPDATE users u
SET broadcast_seen_at = GREATEST(COALESCE(u.broadcast_seen_at, u.created_at), b.created_at)
FROM broadcasts b
WHERE u.id = $1
  AND b.id = $2;

-- name: AckBroadcast :execrows
-- Record one account's answer.
--
-- The choice is validated against the message's own options in the statement
-- itself: an answer that is not on the list matches no row and writes nothing,
-- so the API layer cannot forget the check and a crafted request cannot invent
-- an option. The expiry is tested for the same reason — a question that has
-- ended is not answerable.
--
-- ON CONFLICT DO NOTHING: the first answer stands. An offer that could be
-- retracted and re-answered would make its own tally a moving number, and the
-- panel shows the reader what they chose rather than offering the buttons
-- again.
INSERT INTO broadcast_acks (broadcast_id, user_id, choice)
SELECT b.id, sqlc.arg(user_id), sqlc.arg(choice)
FROM broadcasts b
WHERE b.id = sqlc.arg(broadcast_id)
  AND b.choices @> ARRAY [sqlc.arg(choice)::text]
  AND (b.expires_at IS NULL OR b.expires_at > now())
ON CONFLICT DO NOTHING;

-- name: ListBroadcasts :many
-- The operator's own view on /system: what has been sent, newest first,
-- including messages that have already ended.
SELECT b.id,
       b.created_at,
       b.body,
       b.link,
       b.choices,
       b.expires_at,
       a.username AS actor_username,
       (SELECT count(*) FROM broadcast_acks k WHERE k.broadcast_id = b.id) AS answers
FROM broadcasts b
         LEFT JOIN users a ON a.id = b.actor_id
ORDER BY b.created_at DESC
LIMIT $1;

-- name: BroadcastTallies :many
-- How each message was answered, for the whole page of them in one read.
--
-- There is deliberately no denominator. "How many accounts could have
-- answered" is a scan of the users table for each row, for a number that moves
-- every time somebody registers; the answers themselves are what an offer was
-- sent to find out.
SELECT broadcast_id, choice, count(*) AS answers
FROM broadcast_acks
WHERE broadcast_id = ANY (@ids::bigint[])
GROUP BY broadcast_id, choice
ORDER BY broadcast_id, answers DESC, choice;
