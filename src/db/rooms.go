package db

import (
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// This file is the archive read path behind the permanent room/game permalinks
// (/<room_id>, /<room_id>/<n>, /game/<uuid>) plus the two room-lifecycle
// touch points (all-time ID uniqueness at create, cosmetic close at teardown).
// Every function degrades to a miss/no-op when Postgres is unconfigured, so
// local dev without lio_pg_dsn keeps today's behavior (closed rooms 404).

// RoomIDExists reports whether a room ID is already taken by an archived room.
// Room creation re-rolls candidate IDs through this so a new room can never
// reuse — and thereby hijack — a historical room's permalink. It returns false
// on query errors (logged): a 58^7 collision is vastly less likely than a
// transient DB hiccup, and room creation must never block on the archive.
func RoomIDExists(id string) bool {
	if Pool == nil {
		return false
	}
	ctx, cancel := Ctx()
	defer cancel()
	exists, err := gen.New(Pool).RoomIDExists(ctx, id)
	if err != nil {
		util.Error(str.CDB, "room id existence check failed id=%s error=%s",
			id, err.Error())
		return false
	}
	return exists
}

// MarkRoomClosed stamps the room's cosmetic closed_at marker at teardown. A
// no-op for rooms that never archived a game (no row exists) and when Postgres
// is unconfigured; the read path never depends on closed_at (liveness is
// decided by the in-memory room registry), so a lost close is harmless.
func MarkRoomClosed(id string) {
	if Pool == nil {
		return
	}
	ctx, cancel := Ctx()
	defer cancel()
	if err := gen.New(Pool).CloseRoom(ctx, id); err != nil {
		util.Error(str.CDB, "room close mark failed id=%s error=%s",
			id, err.Error())
	}
}

// GetArchivedRoom fetches a room (match) row by its public room ID. Returns
// found=false on a miss or when Postgres is unconfigured.
func GetArchivedRoom(id string) (gen.Room, bool, error) {
	if Pool == nil {
		return gen.Room{}, false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	room, err := gen.New(Pool).GetRoom(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Room{}, false, nil
	}
	if err != nil {
		return gen.Room{}, false, err
	}
	return room, true, nil
}

// ListRoomGames returns all of a room's archived games ordered by their match
// ordinal (game_index). Empty when Postgres is unconfigured.
func ListRoomGames(id string) ([]gen.Game, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	return gen.New(Pool).ListRoomGames(ctx, id)
}

// GetRoomGameByIndex fetches one archived game of a room by its 1-based match
// ordinal. Returns found=false on a miss or when Postgres is unconfigured.
func GetRoomGameByIndex(roomID string, gameIndex int16) (gen.Game, bool, error) {
	if Pool == nil {
		return gen.Game{}, false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	g, err := gen.New(Pool).GetRoomGameByIndex(ctx, gen.GetRoomGameByIndexParams{
		RoomID:    roomID,
		GameIndex: gameIndex,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Game{}, false, nil
	}
	if err != nil {
		return gen.Game{}, false, err
	}
	return g, true, nil
}

// GetGameByUUID fetches a single archived game by its global game UUID.
// Returns found=false on a miss, an unparseable UUID, or when Postgres is
// unconfigured.
func GetGameByUUID(id string) (gen.Game, bool, error) {
	if Pool == nil {
		return gen.Game{}, false, nil
	}
	gameUUID, err := uuid.Parse(id)
	if err != nil {
		return gen.Game{}, false, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	g, err := gen.New(Pool).GetGameByUUID(ctx, gameUUID)
	if errors.Is(err, pgx.ErrNoRows) {
		return gen.Game{}, false, nil
	}
	if err != nil {
		return gen.Game{}, false, err
	}
	return g, true, nil
}

// H2H is the all-time head-to-head record between two accounts: each side's
// cumulative score (win = 1, draw = ½) across every game they've played against
// each other, and the total game count. A zero record (Games == 0) means there
// is no rivalry to show.
type H2H struct {
	AScore float64
	BScore float64
	Games  int64
}

// HeadToHead returns the all-time head-to-head record between two accounts (by
// user id), the score keyed A/B to the argument order. Both ids must be non-nil
// distinct accounts — a nil id (an anonymous or bot seat), a self-match, or an
// unconfigured Postgres yields a zero record, which callers read as "nothing to
// show" (only persistent accounts have a durable rivalry).
func HeadToHead(a, b *int64) H2H {
	if Pool == nil || a == nil || b == nil || *a == *b {
		return H2H{}
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).HeadToHead(ctx, gen.HeadToHeadParams{UserA: a, UserB: b})
	if err != nil {
		util.Error(str.CDB, "head-to-head lookup failed: %s", err.Error())
		return H2H{}
	}
	return H2H{AScore: row.AScore, BScore: row.BScore, Games: row.Games}
}

// BotH2H is one account's all-time record against a single bot persona: the
// user's cumulative score, the bot's, and the game count. A zero record
// (Games == 0) means nothing to show.
type BotH2H struct {
	UserScore float64
	BotScore  float64
	Games     int64
}

// HeadToHeadVsBot returns a logged-in account's all-time record against a bot
// persona (by resolved persona key). A nil user id (anonymous seat), an empty
// persona, or unconfigured Postgres yields a zero record — only persistent
// accounts accrue a bot rivalry. The caller resolves a NULL/"" bot_persona to
// the Queen's key before calling, so legacy games tally under the right persona.
func HeadToHeadVsBot(userID *int64, persona string) BotH2H {
	if Pool == nil || userID == nil || persona == "" {
		return BotH2H{}
	}
	ctx, cancel := Ctx()
	defer cancel()
	row, err := gen.New(Pool).HeadToHeadVsBot(ctx, gen.HeadToHeadVsBotParams{
		UserID:  userID,
		Persona: &persona,
	})
	if err != nil {
		util.Error(str.CDB, "bot head-to-head lookup failed: %s", err.Error())
		return BotH2H{}
	}
	return BotH2H{UserScore: row.UserScore, BotScore: row.BotScore, Games: row.Games}
}

// ListGameMoveTimes returns an archived game's per-ply timing in ply order,
// nil when the game predates per-move timing (or Postgres is unconfigured).
// Plies are timed all-or-nothing at archive time (BuildPlies), so a NULL on
// any ply reads as an untimed game.
func ListGameMoveTimes(gameRef int32) ([]game.MoveTime, error) {
	if Pool == nil {
		return nil, nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListGameMoveTimes(ctx, gameRef)
	if err != nil {
		return nil, err
	}
	times := make([]game.MoveTime, 0, len(rows))
	for _, r := range rows {
		if r.ClockMs == nil || r.MoveMs == nil {
			return nil, nil
		}
		times = append(times, game.MoveTime{
			ThinkMs: int64(*r.MoveMs),
			ClockMs: int64(*r.ClockMs),
		})
	}
	return times, nil
}

// ListGameMoveEvals returns an archived game's cached per-ply engine evals
// (white-positive centipawns, indexed ply-1), sparse: nil entries are
// positions the background evaluator hasn't reached. Returns nil (no error)
// when Postgres is unconfigured, no ply has an eval yet, or the rows don't
// line up with a contiguous 1..N ply sequence — the archive eval bar simply
// doesn't render then.
func ListGameMoveEvals(gameRef int32) []*int16 {
	if Pool == nil {
		return nil
	}
	ctx, cancel := Ctx()
	defer cancel()
	rows, err := gen.New(Pool).ListGameMoveEvals(ctx, gameRef)
	if err != nil {
		util.Error(str.CDB, "move evals lookup failed game=%d: %s", gameRef, err.Error())
		return nil
	}
	evals := make([]*int16, len(rows))
	any := false
	for i, r := range rows {
		if int(r.Ply) != i+1 {
			return nil // non-contiguous plies: don't guess at alignment
		}
		if r.EvalCp != nil {
			evals[i] = r.EvalCp
			any = true
		}
	}
	if !any {
		return nil
	}
	return evals
}
