-- +goose Up

-- Player feedback: the "tell us what's wrong, or what's working" channel behind
-- the prompt in the profile popover.
--
-- Deliberately not a second reports table. A report is an accusation about an
-- account and needs both parties, a duplicate guard, and a resolution; feedback
-- is a message about the *site*, has exactly one party, and is finished the
-- moment somebody has read it. Folding the two together would have meant a
-- reports row whose target is nullable and whose category set means two
-- different things depending on which it is.
CREATE TABLE feedback (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Cascades like reports do: an account that is gone should not leave rows
    -- pointing at a user that no longer exists. Feedback is product signal, not
    -- a permanent record of anyone's conduct, so nothing here needs to outlive
    -- the person who wrote it.
    user_id    BIGINT      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- What kind of thing this is. 'problem' and 'praise' are the two the prompt
    -- names; 'idea' exists because a feedback box without one collects feature
    -- requests filed as problems, which then read as a broken site.
    kind       TEXT        NOT NULL CHECK (kind IN ('problem', 'praise', 'idea')),
    body       TEXT        NOT NULL CHECK (body <> ''),
    -- The path the visitor was on when they wrote it, captured client-side. The
    -- single most useful piece of context for reproducing a problem, and free:
    -- "the clock jumps" is unactionable, "the clock jumps, on /Ab3xY9" is not.
    path       TEXT        NOT NULL DEFAULT '',
    -- Read state is site-wide rather than per-moderator: the unread badge asks
    -- "is there anything nobody has looked at yet", and the same question with a
    -- per-person answer would need a join table to say something less useful.
    read_at    TIMESTAMPTZ,
    read_by    BIGINT REFERENCES users (id)
);

-- the badge query on every moderator page render: count of unread. Partial, so
-- it stays the size of the backlog rather than the size of the table.
CREATE INDEX feedback_unread_idx ON feedback (created_at) WHERE read_at IS NULL;
-- the inbox read: newest first, the order it is actually worked
CREATE INDEX feedback_recent_idx ON feedback (created_at DESC);
-- the per-account submission cap ("how many has this account filed today?")
CREATE INDEX feedback_user_idx ON feedback (user_id, created_at DESC);

-- +goose Down
DROP TABLE IF EXISTS feedback;
