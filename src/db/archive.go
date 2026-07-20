package db

import (
	"context"
	"errors"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/game"
)

// GameRecord is the full archive row for one finished game, assembled at the
// storeGame seam: the room-level fields are captured under stateMu in
// tryGameOver (scores, race-to, reason), the rest filled from the finished game
// copy in storeGame. It is decoupled from the generated types so the room
// package never imports db/gen.
type GameRecord struct {
	// room-level (captured under stateMu in tryGameOver)
	RoomID        string
	Creator       string
	CreatorUserID *int64
	RaceTo        int
	Rated         bool
	WhiteScore    float64
	BlackScore    float64
	Reason        string

	// account identity of each seat, captured under stateMu alongside the
	// scores (arch/ACCOUNTS_AUTH_RATINGS.md Phase 2). UserIDs stamp the new
	// nullable FK columns (nil = anon/bot); WhiteName/BlackName are the
	// PGN-ready display names ("BOT"/"Anonymous"/<username>).
	WhiteUserID *int64
	BlackUserID *int64
	WhiteName   string
	BlackName   string

	// game-level (filled from the finished game copy in storeGame)
	GameID       string
	StartTs      time.Time
	EndTs        time.Time
	WhiteUID     string
	BlackUID     string
	VariantName  string
	VariantGroup string
	Casual       bool
	Outcome      string
	Method       int16
	StartingOFEN string
	Moves        []byte
	PGNObjectKey string
}

// PlyRecord is one ply of the derived move/position analytics index: the packed
// move, the position reached after it (its clock-independent hash + OFEN), and
// the ply's timing — remaining clock after the move (ClockMs) and think time as
// charged (MoveMs). The timing pointers are nil when unrecorded (the PGN
// backfill; games predating per-move timing).
type PlyRecord struct {
	Ply     int16
	Mv      int16
	PosHash []byte
	PosOFEN string
	ClockMs *int32
	MoveMs  *int32
}

// BuildPlies encodes a finished game's move list into the compact packed blob
// (~1 byte/ply after BYTEA storage) and the per-ply analytics rows: the position
// reached after each move, keyed by its clock-independent hash. It is the single
// source of the archive's move/position encoding — both the live seam and the
// backfill call it, so a replayed game hashes byte-for-byte identically to a
// live one (otherwise the deduped positions table would fragment). times is the
// per-ply timing recorded by the room, parallel to the move list; nil (or a
// desynced length, which should never happen) archives the plies untimed.
func BuildPlies(g *octad.Game, times []game.MoveTime) ([]byte, []PlyRecord) {
	positions := g.Positions()
	moves := g.Moves()
	if len(times) != len(moves) {
		times = nil
	}
	blob := make([]byte, 0, len(moves)*2)
	plies := make([]PlyRecord, 0, len(moves))
	for i, m := range moves {
		packed := game.PackMove(m)
		blob = append(blob, byte(packed>>8), byte(packed))
		pos := positions[i+1] // position after ply i (positions[0] is the start)
		h := pos.Hash()
		rec := PlyRecord{
			Ply:     int16(i + 1),
			Mv:      packed,
			PosHash: h[:],
			PosOFEN: pos.String(),
		}
		if times != nil {
			clockMs := int32(times[i].ClockMs)
			moveMs := int32(times[i].ThinkMs)
			rec.ClockMs = &clockMs
			rec.MoveMs = &moveMs
		}
		plies = append(plies, rec)
	}
	return blob, plies
}

// ArchiveGame writes a finished game and its per-ply analytics rows (deduped
// positions + moves) in a single transaction. It is a no-op (nil) when Postgres
// is unconfigured — local dev without lio_pg_dsn degrades to object-storage
// archival only, exactly like store. Callers run this off the hot path (the
// storeGame background goroutine), so the transaction never gates game play.
// The returned *RatingResult is non-nil only when the game was rated and moved
// both players' ratings — the room broadcasts it so the game-over popup shows
// each side's delta and the clocks refresh for a rematch.
func ArchiveGame(ctx context.Context, rec GameRecord, plies []PlyRecord) (*RatingResult, error) {
	_, res, err := archiveGame(ctx, rec, plies, false)
	return res, err
}

// ArchiveGameIfNew archives a game only when no row with the same
// pgn_object_key already exists, reporting whether a row was inserted. This is
// the archive backfill's idempotency key: it both skips games already recorded
// by the live seam and makes a partial backfill safe to re-run.
func ArchiveGameIfNew(ctx context.Context, rec GameRecord, plies []PlyRecord) (bool, error) {
	inserted, _, err := archiveGame(ctx, rec, plies, true)
	return inserted, err
}

// archiveGame is the shared transactional core. When ifNew is set, the game
// insert is a no-op on a pgn_object_key conflict (returning inserted=false); a
// conflicting game contributes neither its row nor its moves.
func archiveGame(ctx context.Context, rec GameRecord, plies []PlyRecord, ifNew bool) (bool, *RatingResult, error) {
	if Pool == nil {
		return false, nil, nil
	}

	gameUUID, err := uuid.Parse(rec.GameID)
	if err != nil {
		return false, nil, err
	}

	tx, err := Pool.Begin(ctx)
	if err != nil {
		return false, nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once Commit succeeds

	q := gen.New(tx)

	// game_index is the 1-based ordinal within the room (the <n> in the
	// /<room_id>/<n> permalink), derived here rather than in the room actor so
	// it survives restarts and same-room fresh matches (player result history
	// resets when a decided race-to match rematches). Room-less rows (backfill)
	// keep index 0. games_room_game_idx UNIQUE backstops the concurrent case.
	var gameIndex int16
	if rec.RoomID != "" {
		gameIndex, err = q.NextGameIndex(ctx, rec.RoomID)
		if err != nil {
			return false, nil, err
		}
	}

	// Glicko-2 rating update for a finished rated game, in the same transaction
	// (arch/ACCOUNTS_AUTH_RATINGS.md Phase 5). Only the live archive path applies
	// ratings — the backfill (ifNew) replays history and must never mutate them.
	// Run before the game insert so the per-seat rating-at-game + delta snapshot
	// onto the row (the archive's "rating at the time of the game").
	var ratingRes *RatingResult
	if !ifNew {
		ratingRes, err = applyRatingUpdate(ctx, q, rec)
		if err != nil {
			return false, nil, err
		}
	}
	var whiteRating, blackRating *string
	var whiteRatingDelta, blackRatingDelta *int16
	if ratingRes != nil {
		wr, br := ratingRes.White.AtGame, ratingRes.Black.AtGame
		wd, bd := int16(ratingRes.White.Delta), int16(ratingRes.Black.Delta)
		whiteRating, blackRating = &wr, &br
		whiteRatingDelta, blackRatingDelta = &wd, &bd
	}

	params := gen.InsertGameParams{
		GameID:           gameUUID,
		StartTs:          ts(rec.StartTs),
		EndTs:            ts(rec.EndTs),
		RaceTo:           int32(rec.RaceTo),
		WhiteScore:       float32(rec.WhiteScore),
		BlackScore:       float32(rec.BlackScore),
		Method:           rec.Method,
		Casual:           rec.Casual,
		RoomID:           rec.RoomID,
		CreatorUid:       rec.Creator,
		WhiteUid:         rec.WhiteUID,
		BlackUid:         rec.BlackUID,
		VariantName:      rec.VariantName,
		VariantGroup:     rec.VariantGroup,
		Outcome:          rec.Outcome,
		Reason:           rec.Reason,
		StartingOfen:     rec.StartingOFEN,
		Moves:            rec.Moves,
		PgnObjectKey:     rec.PGNObjectKey,
		GameIndex:        gameIndex,
		WhiteUserID:      rec.WhiteUserID,
		BlackUserID:      rec.BlackUserID,
		CreatorUserID:    rec.CreatorUserID,
		Rated:            rec.Rated,
		WhiteRating:      whiteRating,
		BlackRating:      blackRating,
		WhiteRatingDelta: whiteRatingDelta,
		BlackRatingDelta: blackRatingDelta,
	}

	var gameRef int32
	if ifNew {
		// InsertGameIfNew shares InsertGame's exact column set/order, so the two
		// generated param structs are identical and convert directly.
		gameRef, err = q.InsertGameIfNew(ctx, gen.InsertGameIfNewParams(params))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil, nil // already archived under this pgn_object_key
		}
	} else {
		gameRef, err = q.InsertGame(ctx, params)
	}
	if err != nil {
		return false, nil, err
	}

	// keep the room (match) row current in the same transaction: inserted on
	// the match's first archived game, then the "as of latest game" fields
	// refresh with each one — so the room permalink is fully served even if the
	// room's cosmetic close marker never lands (crash).
	if rec.RoomID != "" {
		if err := q.UpsertRoom(ctx, gen.UpsertRoomParams{
			RoomID:        rec.RoomID,
			FirstGameTs:   ts(rec.StartTs),
			LastGameTs:    ts(rec.EndTs),
			RaceTo:        int32(rec.RaceTo),
			GameCount:     int32(gameIndex),
			Casual:        rec.Casual,
			CreatorUid:    rec.Creator,
			WhiteUid:      rec.WhiteUID,
			BlackUid:      rec.BlackUID,
			WhiteScore:    float32(rec.WhiteScore),
			BlackScore:    float32(rec.BlackScore),
			VariantName:   rec.VariantName,
			VariantGroup:  rec.VariantGroup,
			CreatorUserID: rec.CreatorUserID,
			WhiteUserID:   rec.WhiteUserID,
			BlackUserID:   rec.BlackUserID,
			Rated:         rec.Rated,
		}); err != nil {
			return false, nil, err
		}
	}

	for _, p := range plies {
		positionID, err := q.UpsertPosition(ctx, gen.UpsertPositionParams{
			Hash: p.PosHash,
			Ofen: p.PosOFEN,
		})
		if err != nil {
			return false, nil, err
		}
		if err := q.InsertMove(ctx, gen.InsertMoveParams{
			GameRef:    gameRef,
			PositionID: positionID,
			ClockMs:    p.ClockMs,
			MoveMs:     p.MoveMs,
			Ply:        p.Ply,
			Mv:         p.Mv,
		}); err != nil {
			return false, nil, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, nil, err
	}
	gamesTotal.Add(1)
	return true, ratingRes, nil
}

// ts wraps a time.Time as a valid pgtype.Timestamptz.
func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
