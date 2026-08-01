-- +goose Up

-- Broadcasts: one message written once and read by every account
-- (arch/NOTIFICATIONS.md, *A broadcast is one row*).
--
-- The notifications table cannot express this. A message for every account
-- would be one row per account: the same sentence stored thousands of times,
-- a write that grows with the user table, and a backfill for every account
-- created afterwards. So the message is stored one time here, and each
-- account's read state is derived rather than stored — see the watermark below.
--
-- This does not replace the site notice in the settings table. That banner
-- reaches an anonymous visitor, who has no bell and no read state; a broadcast
-- reaches accounts, keeps per-account read state, and can require an answer.
-- An announcement that must reach everybody who opens the site is still the
-- banner.
CREATE TABLE broadcasts (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The admin who sent it. Recorded for the audit trail, and deliberately not
    -- rendered to the recipient: a broadcast speaks for the site, exactly like
    -- the operator message composer does.
    --
    -- SET NULL for the same reason notifications.actor_id is: the message
    -- outlives the account that wrote it.
    actor_id   BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    body       TEXT        NOT NULL CHECK (body <> ''),
    -- Where the row goes when somebody clicks it, empty for a message with no
    -- destination. A path on this site, built by the handler.
    link       TEXT        NOT NULL DEFAULT '',
    -- The answers this message demands, in the order they are shown, or NULL
    -- for an ordinary announcement. A non-NULL value makes the row an
    -- acknowledgement: it does not clear by being seen, only by being answered,
    -- and the answer lands in broadcast_acks.
    --
    -- An array rather than a boolean because the useful cases differ in their
    -- options, not only in whether they have any: one "OK" for a change
    -- somebody must confirm they saw, "Yes"/"No" for an offer worth counting.
    -- The empty array is refused, because a row that can never be answered
    -- could never be cleared.
    choices    TEXT[] CHECK (choices IS NULL OR cardinality(choices) > 0),
    -- When the message stops being shown. NULL means it runs until it is
    -- retired, which is this column set to now().
    --
    -- It bounds an unanswered acknowledgement as well. Without it a question
    -- nobody answers would sit in their panel for the life of the account.
    expires_at TIMESTAMPTZ
);

-- One account's answer to one broadcast. Sparse on purpose: a row exists only
-- where somebody actually answered, so an announcement nobody has to answer
-- stores nothing at all here, and an offer stores exactly as many rows as it
-- got replies.
--
-- This is also the tally an operator reads back. An "offer" is only worth
-- sending if the answers can be counted, and the primary key is the group-by.
CREATE TABLE broadcast_acks (
    broadcast_id BIGINT      NOT NULL REFERENCES broadcasts (id) ON DELETE CASCADE,
    user_id      BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The chosen option, stored as the label rather than as an index. An index
    -- becomes meaningless if the choices are ever edited, and a stored answer
    -- has to stay readable years later.
    choice       TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (broadcast_id, user_id)
);

-- The read watermark: everything broadcast on or before this moment has been
-- seen by this account.
--
-- One timestamp instead of a row per (account, broadcast) pair. That is the
-- whole reason a broadcast is affordable: reading is the common event, and it
-- must not write a row for every message the reader has ever been sent.
--
-- NULL means "never read one", which resolves to the account's own created_at
-- — so an account that registers today does not open its bell to a year of
-- announcements it was not around for. Every read of this column pairs it with
-- COALESCE(broadcast_seen_at, created_at) for that reason.
--
-- The watermark deliberately does not clear an unanswered acknowledgement.
-- Those are keyed off broadcast_acks instead, so "I have seen these" and "I
-- have answered this" stay separate events — the same rule that leaves a live
-- challenge unread when the bell is opened.
ALTER TABLE users
    ADD COLUMN broadcast_seen_at TIMESTAMPTZ;

-- The same acknowledgement flag, on a message addressed to one account.
--
-- It is a property of a notification rather than of a broadcast, because the
-- question "did this person accept?" is worth asking of one player as well as
-- of everybody — an operator answering a report can ask the player to confirm
-- they have read the decision. The client renders both from one row shape, so
-- the columns match the broadcast ones exactly.
ALTER TABLE notifications
    ADD COLUMN choices TEXT[] CHECK (choices IS NULL OR cardinality(choices) > 0);
-- What the recipient chose, NULL while the question is outstanding. The answer
-- lives on the row itself here: a notification already belongs to exactly one
-- account, so it needs no join table the way a broadcast does.
ALTER TABLE notifications
    ADD COLUMN response TEXT;

-- +goose Down
ALTER TABLE notifications
    DROP COLUMN IF EXISTS response;
ALTER TABLE notifications
    DROP COLUMN IF EXISTS choices;
ALTER TABLE users
    DROP COLUMN IF EXISTS broadcast_seen_at;
DROP TABLE IF EXISTS broadcast_acks;
DROP TABLE IF EXISTS broadcasts;
