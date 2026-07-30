-- +goose Up

-- Per-account notifications: the messages behind the bell in the header
-- (arch/NOTIFICATIONS.md).
--
-- This table holds the durable kinds only. A message that expires does not
-- belong here yet; Phase 2 adds the challenge kind and an expires_at column.
--
-- The site notice in the settings table stays where it is. That notice goes to
-- everybody and holds no per-account state, so one row for each account would
-- store the same sentence thousands of times.
--
-- Unread feedback also stays out of this table. Its read state is site-wide on
-- purpose (see 00020_feedback.sql): the badge answers "did anybody read this
-- yet". The panel shows that count as a derived item for a moderator.
CREATE TABLE notifications (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- The account that receives the message. It cascades: a notification has no
    -- meaning after its reader is gone.
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind       TEXT        NOT NULL CHECK (kind IN ('mod_action', 'milestone', 'system')),
    -- The account that caused the message: the moderator, or later the player
    -- who sends a challenge. NULL for a message from the site itself.
    --
    -- SET NULL, not CASCADE. The recipient owns this row. If an actor deletes
    -- their account, the recipient must keep their history, so the delete
    -- clears the reference and the renderer shows the message with no link.
    --
    -- The name is not copied into body for the same reason a rendered name is
    -- always read from users: a person can change their username, and a copy
    -- becomes wrong the moment they do.
    actor_id   BIGINT      REFERENCES users (id) ON DELETE SET NULL,
    body       TEXT        NOT NULL CHECK (body <> ''),
    -- Where the row goes when somebody clicks it. Empty for a message with no
    -- destination. Always a path on this site, never a full URL; the writers
    -- build it, so nothing from a client reaches this column.
    link       TEXT        NOT NULL DEFAULT '',
    read_at    TIMESTAMPTZ
);

-- The badge count, and the only query that runs for each socket connect. The
-- index is partial, so it stays the size of one account's unread backlog.
CREATE INDEX notifications_unread_idx ON notifications (user_id)
    WHERE read_at IS NULL;
-- The panel list: one account's rows, newest first.
CREATE INDEX notifications_recent_idx ON notifications (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS notifications;
