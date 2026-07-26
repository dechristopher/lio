-- +goose Up

-- Player reports (arch/ADMIN_MODERATION.md Phase 4): the one moderation surface
-- ordinary players touch. Everything else in this feature is operator-facing;
-- this is the path by which a moderator learns there is anything to look at.
--
-- Reports are about *accounts*, so both parties are user ids rather than session
-- uids: an anonymous opponent cannot be reported (there is nothing to sanction)
-- and a bot has no account at all. game_id is the evidence when the report comes
-- out of a specific game, and NULL when it does not.
CREATE TABLE reports (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Both cascade: deleting an account should not leave reports pointing at a
    -- row that no longer exists. This deliberately differs from mod_actions,
    -- which pins its parties — the audit log is a permanent record of what
    -- moderators did, while a report is a transient request for attention.
    reporter_user_id BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    target_user_id   BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    game_id          UUID        REFERENCES games (game_id),
    category         TEXT        NOT NULL CHECK (category IN
                                   ('cheating', 'sandbagging', 'stalling', 'username', 'other')),
    note             TEXT        NOT NULL DEFAULT '',
    status           TEXT        NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'closed')),
    resolved_at      TIMESTAMPTZ,
    resolved_by      BIGINT      REFERENCES users (id),
    -- what the moderator decided, free text, shown in the closed list
    resolution       TEXT,
    -- a report must not be able to name itself as its own reporter
    CONSTRAINT reports_not_self CHECK (reporter_user_id <> target_user_id)
);

-- One open report per reporter→target pair. Without this a single aggrieved
-- player can bury the queue in reports about one opponent; with it, a second
-- report only becomes possible once the first has been dealt with. Partial, so
-- the same pair can legitimately recur after each resolution.
CREATE UNIQUE INDEX reports_open_unique
    ON reports (reporter_user_id, target_user_id) WHERE status = 'open';

-- the queue read: open reports, oldest first (the queue is worked in order)
CREATE INDEX reports_open_idx ON reports (created_at) WHERE status = 'open';
-- "does this account have anything against it?", for the player page
CREATE INDEX reports_target_idx ON reports (target_user_id, status);

-- +goose Down
DROP TABLE IF EXISTS reports;
