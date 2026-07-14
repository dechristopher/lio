package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
)

// GameRecord is the full archive row for one finished game, assembled at the
// storeGame seam: the room-level fields are captured under stateMu in
// tryGameOver (scores, race-to, reason), the rest filled from the finished game
// copy in storeGame. It is decoupled from the generated types so the room
// package never imports db/gen.
type GameRecord struct {
	// room-level (captured under stateMu in tryGameOver)
	RoomID     string
	Creator    string
	RaceTo     int
	WhiteScore float64
	BlackScore float64
	Reason     string

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
// move and the position reached after it (its clock-independent hash + OFEN).
type PlyRecord struct {
	Ply     int16
	Mv      int16
	PosHash []byte
	PosOFEN string
	ClockMs *int32
}

// ArchiveGame writes a finished game and its per-ply analytics rows (deduped
// positions + moves) in a single transaction. It is a no-op (nil) when Postgres
// is unconfigured — local dev without lio_pg_dsn degrades to object-storage
// archival only, exactly like store. Callers run this off the hot path (the
// storeGame background goroutine), so the transaction never gates game play.
func ArchiveGame(ctx context.Context, rec GameRecord, plies []PlyRecord) error {
	if Pool == nil {
		return nil
	}

	gameUUID, err := uuid.Parse(rec.GameID)
	if err != nil {
		return err
	}

	tx, err := Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once Commit succeeds

	q := gen.New(tx)

	gameRef, err := q.InsertGame(ctx, gen.InsertGameParams{
		GameID:       gameUUID,
		StartTs:      ts(rec.StartTs),
		EndTs:        ts(rec.EndTs),
		RaceTo:       int32(rec.RaceTo),
		WhiteScore:   float32(rec.WhiteScore),
		BlackScore:   float32(rec.BlackScore),
		Method:       rec.Method,
		Casual:       rec.Casual,
		RoomID:       rec.RoomID,
		CreatorUid:   rec.Creator,
		WhiteUid:     rec.WhiteUID,
		BlackUid:     rec.BlackUID,
		VariantName:  rec.VariantName,
		VariantGroup: rec.VariantGroup,
		Outcome:      rec.Outcome,
		Reason:       rec.Reason,
		StartingOfen: rec.StartingOFEN,
		Moves:        rec.Moves,
		PgnObjectKey: rec.PGNObjectKey,
	})
	if err != nil {
		return err
	}

	for _, p := range plies {
		positionID, err := q.UpsertPosition(ctx, gen.UpsertPositionParams{
			Hash: p.PosHash,
			Ofen: p.PosOFEN,
		})
		if err != nil {
			return err
		}
		if err := q.InsertMove(ctx, gen.InsertMoveParams{
			GameRef:    gameRef,
			PositionID: positionID,
			ClockMs:    p.ClockMs,
			Ply:        p.Ply,
			Mv:         p.Mv,
		}); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

// ts wraps a time.Time as a valid pgtype.Timestamptz.
func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
