-- +goose Up

-- Site administration & moderation (arch/ADMIN_MODERATION.md).
--
-- role is a single ordered column rather than its own table — the deliberate
-- opposite of the titles decision (00014). A title is pure display data whose
-- wording should be editable at will; a role is *code-coupled*: the server has
-- to know what 'mod' may do, so a role that isn't in the code is meaningless
-- and the CHECK says exactly that. The ladder is player < mod < admin (see
-- src/role): mods sanction players, admins additionally set roles and site
-- controls. The first admin is bootstrapped by hand —
--   UPDATE users SET role = 'admin' WHERE lower(username) = lower('someone');
-- — no UI path can mint one.
ALTER TABLE users
    ADD COLUMN role TEXT NOT NULL DEFAULT 'player'
        CHECK (role IN ('player', 'mod', 'admin'));

-- banned_until NULL means "not banned"; 'infinity' means permanent. One column
-- and one `banned_until > now()` comparison therefore covers both temporary and
-- permanent sanctions, and an expired ban lapses on its own with no sweeper.
-- A ban is an *identity* sanction, not an access one: anonymous play is
-- first-class here, so this removes the account, its rated play and its
-- displayed ratings — not the person's ability to open the site.
ALTER TABLE users
    ADD COLUMN banned_until TIMESTAMPTZ;
ALTER TABLE users
    ADD COLUMN ban_reason TEXT;

-- Banned accounts are a rounding error of the table, so the "is this account
-- banned" read rides a partial index like users_title_id_idx does.
CREATE INDEX users_banned_until_idx ON users (banned_until) WHERE banned_until IS NOT NULL;

-- Append-only record of every privileged action. The users columns above are
-- the cheap enforcement read; this table is the truth about what happened, and
-- it is never updated or deleted. reason is NOT NULL on purpose: every action a
-- moderator takes — including granting a title — has to be justified in
-- writing, and every moderator can read every other moderator's entries. There
-- is no silent moderation.
CREATE TABLE mod_actions (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- who acted. Never deleted out from under the log: an account that has
    -- moderated cannot be hard-deleted without addressing this reference.
    actor_user_id  BIGINT      NOT NULL REFERENCES users (id),
    -- who it was done to; NULL for site-level actions (settings changes),
    -- which have no target account.
    target_user_id BIGINT      REFERENCES users (id),
    -- ban | unban | title | role | rename | setting | report
    action         TEXT        NOT NULL,
    -- action-specific before/after payload, e.g. {"from":"player","to":"mod"}
    -- or {"until":"2026-08-01T00:00:00Z"}. JSONB so the shape can vary per
    -- action without a column per verb.
    detail         JSONB       NOT NULL DEFAULT '{}',
    reason         TEXT        NOT NULL
);

-- the per-account history shown on a player's page (mods only)
CREATE INDEX mod_actions_target_idx ON mod_actions (target_user_id, created_at DESC);
-- the global audit feed in /mod
CREATE INDEX mod_actions_created_idx ON mod_actions (created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS mod_actions;

DROP INDEX IF EXISTS users_banned_until_idx;

ALTER TABLE users
    DROP COLUMN IF EXISTS ban_reason;
ALTER TABLE users
    DROP COLUMN IF EXISTS banned_until;
ALTER TABLE users
    DROP COLUMN IF EXISTS role;
