-- +goose Up

-- Direct challenges (arch/NOTIFICATIONS.md Phase 2): one player invites another
-- to a game, and the invitation arrives as a notification.
--
-- The challenge itself is not stored here. A direct challenge is a room with an
-- invited account, created up front, and the room is the authority on whether it
-- is still open. This table holds only the message that told somebody about it.
-- That is why there is no state column: accepted, declined and expired are all
-- answered by the room, and a second copy of that answer here could disagree
-- with it.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_kind_check
        CHECK (kind IN ('mod_action', 'milestone', 'system', 'challenge'));

-- When the invitation stops being worth acting on. The panel shows the time
-- left, and stops offering Accept once it passes.
--
-- It is a display bound, not the authority. A waiting room dies about a minute
-- after its creator leaves the page (reconnectGrace in room/handlers.go), so a
-- challenge is usually gone well before this stamp; the room-gone redirect
-- handles that case, exactly as it does for a stale open challenge. The stamp
-- exists so the panel can stop offering an action that is certainly dead, and so
-- a challenge row ages out of looking actionable.
--
-- NULL for every other kind, which is why this is not NOT NULL: a moderation
-- decision and a rating record do not expire.
ALTER TABLE notifications
    ADD COLUMN expires_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE notifications
    DROP COLUMN IF EXISTS expires_at;
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_kind_check
        CHECK (kind IN ('mod_action', 'milestone', 'system'));
