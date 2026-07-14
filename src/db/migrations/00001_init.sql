-- +goose Up

-- games: one row per finished game — the durable relational archive. The PGN
-- blob keeps its home in object storage; this table holds the object key plus
-- queryable metadata. Fixed columns are ordered widest-first to minimise row
-- padding. white_uid/black_uid stay plain TEXT for now (cookie identity); they
-- pick up FKs to a users table when accounts land.
CREATE TABLE games (
    id             INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    game_id        UUID        NOT NULL UNIQUE,
    start_ts       TIMESTAMPTZ NOT NULL,
    end_ts         TIMESTAMPTZ NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    race_to        INT         NOT NULL DEFAULT 0,
    white_score    REAL        NOT NULL,
    black_score    REAL        NOT NULL,
    method         SMALLINT    NOT NULL,
    casual         BOOLEAN     NOT NULL,
    room_id        TEXT        NOT NULL,
    creator_uid    TEXT        NOT NULL,
    white_uid      TEXT        NOT NULL,
    black_uid      TEXT        NOT NULL,
    variant_name   TEXT        NOT NULL,
    variant_group  TEXT        NOT NULL,
    outcome        TEXT        NOT NULL,
    reason         TEXT        NOT NULL,
    starting_ofen  TEXT        NOT NULL,
    moves          BYTEA       NOT NULL,
    pgn_object_key TEXT        NOT NULL
) WITH (fillfactor = 100);

CREATE INDEX games_white_uid_idx ON games (white_uid);
CREATE INDEX games_black_uid_idx ON games (black_uid);
-- BRIN: games are append-only and start_ts-correlated, so a block-range index
-- is a fraction of a btree's size for time-window scans.
CREATE INDEX games_start_ts_brin ON games USING brin (start_ts);

-- positions: distinct positions deduped by octad's clock-independent
-- Position.Hash(). Each distinct position is stored — and evaluated — once; the
-- eval columns are filled lazily by the background evaluator. Lower fillfactor
-- leaves room for those in-place (HOT) eval updates.
CREATE TABLE positions (
    id           INT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    hash         BYTEA    NOT NULL UNIQUE,
    ofen         TEXT     NOT NULL,
    eval_cp      SMALLINT,
    eval_depth   SMALLINT,
    best_move    SMALLINT,
    evaluated_at TIMESTAMPTZ
) WITH (fillfactor = 90);

-- the evaluator queue: a tiny partial index over just the unevaluated rows.
CREATE INDEX positions_need_eval_idx ON positions (id) WHERE eval_cp IS NULL;

-- moves: one row per ply. game_ref/position_id are compact INT surrogates
-- (never the 16-byte game UUID), the move is a packed SMALLINT, position_id is
-- the position reached after the move. All fixed-width and padding-free.
CREATE TABLE moves (
    game_ref    INT      NOT NULL REFERENCES games (id) ON DELETE CASCADE,
    position_id INT      NOT NULL REFERENCES positions (id),
    clock_ms    INT,
    ply         SMALLINT NOT NULL,
    mv          SMALLINT NOT NULL,
    PRIMARY KEY (game_ref, ply)
) WITH (fillfactor = 100);

CREATE INDEX moves_position_id_idx ON moves (position_id);

-- +goose Down
DROP TABLE IF EXISTS moves;
DROP TABLE IF EXISTS positions;
DROP TABLE IF EXISTS games;
