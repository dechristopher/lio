-- +goose Up

-- The follow graph (arch/FOLLOWING.md): one row per directed edge. "A follows
-- B" is a single fact, and this table stores nothing else about it.
--
-- The pair IS the row. There is no surrogate id column: a generated key would
-- add 8 bytes plus its own unique index to a table whose entire content is two
-- integers, and nothing would ever read it. The primary key doubles as the
-- uniqueness constraint, which is what makes a repeated follow a no-op rather
-- than a duplicate.
--
-- There is likewise no state column. A follow needs no consent, so there is no
-- pending value, and the two real states are the presence and the absence of
-- the row.
CREATE TABLE follows (
    -- The account that follows. CASCADE: outgoing follows have no meaning once
    -- the account holding them is gone.
    follower_id BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- The account being followed. CASCADE for the mirror reason: a follow that
    -- points at a deleted account points at nobody.
    followee_id BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- When the follow happened. It orders both lists (newest first) and is the
    -- only fact about a follow beyond its existence.
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    -- a self-follow is not a thing that can be meant
    CONSTRAINT follows_not_self CHECK (follower_id <> followee_id)
);

-- The reverse direction: "who follows this account", for the followers list and
-- the follower count. The primary key already serves the forward direction —
-- one account's following list, and the membership probe behind the online
-- filter — so this is the only additional index the table needs.
--
-- Neither index carries created_at, though both lists sort by it. Covering that
-- sort would need (follower_id, created_at DESC) and (followee_id, created_at
-- DESC), which would leave the primary key as a third index duplicating the
-- leading column of one of them. A follow list is tens to a few hundred rows;
-- sorting that costs nothing, and the storage does not.
CREATE INDEX follows_followee_idx ON follows (followee_id);

-- +goose Down
DROP TABLE IF EXISTS follows;
