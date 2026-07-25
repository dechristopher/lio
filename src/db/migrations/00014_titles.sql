-- +goose Up

-- Titles become first-class rows instead of a free-form string on the account.
-- A title is now an object with its own identity: the short badge `code` shown
-- beside the username ("GM") and the full `name` the badge renders as its
-- hover tooltip ("Grandmaster"). users.title_id references it, so renaming a
-- title (or fixing its tooltip) is one UPDATE here rather than a rewrite of
-- every account that holds it, and a typo can no longer mint a one-off title.
--
-- There is still no in-app assignment UI, by design: an operator assigns with
--   UPDATE users SET title_id = (SELECT id FROM titles WHERE code = 'GM')
--   WHERE lower(username) = lower('someone');
-- and adds a new title with a plain INSERT into this table.
CREATE TABLE titles (
    id         SMALLINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    code       TEXT        NOT NULL,
    name       TEXT        NOT NULL
);

-- Codes are the display identity and are matched case-insensitively, exactly
-- like usernames: "gm" and "GM" must not coexist as separate titles.
CREATE UNIQUE INDEX titles_code_lower_key ON titles (lower(code));

-- The starter set: site titles plus the chess-style ladder they'd be read
-- against. Rows are ordinary data — add, rename or delete freely.
INSERT INTO titles (code, name)
VALUES ('OG', 'Original Gamer'),
       ('DEV', 'Developer'),
       ('MOD', 'Moderator'),
       ('GM', 'Grandmaster'),
       ('IM', 'International Master'),
       ('FM', 'FIDE Master'),
       ('CM', 'Candidate Master'),
       ('NM', 'National Master'),
       ('WGM', 'Woman Grandmaster'),
       ('WIM', 'Woman International Master'),
       ('WFM', 'Woman FIDE Master'),
       ('WCM', 'Woman Candidate Master'),
       ('LM', 'Lio Master');

-- Preserve any free-form title already assigned that the seed doesn't cover:
-- mint a row for it with the code doubling as its name (an operator can write
-- a real tooltip afterward). DISTINCT ON dedupes case variants within this one
-- statement — ON CONFLICT only guards against the seeded rows above.
INSERT INTO titles (code, name)
SELECT DISTINCT ON (lower(btrim(u.title))) btrim(u.title), btrim(u.title)
FROM users u
WHERE u.title IS NOT NULL
  AND btrim(u.title) <> ''
ORDER BY lower(btrim(u.title))
ON CONFLICT (lower(code)) DO NOTHING;

ALTER TABLE users
    ADD COLUMN title_id SMALLINT REFERENCES titles (id) ON DELETE SET NULL;

UPDATE users u
SET title_id = t.id
FROM titles t
WHERE u.title IS NOT NULL
  AND lower(btrim(u.title)) = lower(t.code);

-- Titled accounts are a rounding error of the table, so the FK's lookup (and
-- the ON DELETE SET NULL scan) rides a partial index instead of a full one.
CREATE INDEX users_title_id_idx ON users (title_id) WHERE title_id IS NOT NULL;

ALTER TABLE users
    DROP COLUMN title;

-- +goose Down
ALTER TABLE users
    ADD COLUMN title TEXT;

UPDATE users u
SET title = t.code
FROM titles t
WHERE u.title_id = t.id;

DROP INDEX IF EXISTS users_title_id_idx;

ALTER TABLE users
    DROP COLUMN IF EXISTS title_id;

DROP TABLE IF EXISTS titles;
