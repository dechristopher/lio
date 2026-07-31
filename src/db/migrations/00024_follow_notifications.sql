-- +goose Up

-- Gaining a follower is worth telling somebody about (arch/FOLLOWING.md Phase
-- 3). It reuses the notifications table whole: one new kind value, the existing
-- socket delivery, the existing panel. There is no new transport and no new
-- client.
--
-- Like 'mod_action' and 'milestone' and unlike 'challenge', this kind is
-- durable: it carries no expires_at, offers no action, and simply waits to be
-- read. The row's actor_id is the new follower, which is what names them in the
-- panel — and names them *currently*, so a later rename does not leave an old
-- name sitting in somebody's list.
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_kind_check
        CHECK (kind IN ('mod_action', 'milestone', 'system', 'challenge', 'follow'));

-- +goose Down
-- Drop any rows of the kind being removed first: the constraint cannot be
-- narrowed back while rows violate it.
DELETE FROM notifications WHERE kind = 'follow';
ALTER TABLE notifications
    DROP CONSTRAINT notifications_kind_check;
ALTER TABLE notifications
    ADD CONSTRAINT notifications_kind_check
        CHECK (kind IN ('mod_action', 'milestone', 'system', 'challenge'));
