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

// BuildPlies encodes a finished game's move list into the compact packed blob
// (~1 byte/ply after BYTEA storage) and the per-ply analytics rows: the position
// reached after each move, keyed by its clock-independent hash. It is the single
// source of the archive's move/position encoding — both the live seam and the
// backfill call it, so a replayed game hashes byte-for-byte identically to a
// live one (otherwise the deduped positions table would fragment).
func BuildPlies(g *octad.Game) ([]byte, []PlyRecord) {
	positions := g.Positions()
	moves := g.Moves()
	blob := make([]byte, 0, len(moves)*2)
	plies := make([]PlyRecord, 0, len(moves))
	for i, m := range moves {
		packed := game.PackMove(m)
		blob = append(blob, byte(packed>>8), byte(packed))
		pos := positions[i+1] // position after ply i (positions[0] is the start)
		h := pos.Hash()
		plies = append(plies, PlyRecord{
			Ply:     int16(i + 1),
			Mv:      packed,
			PosHash: h[:],
			PosOFEN: pos.String(),
		})
	}
	return blob, plies
}

// ArchiveGame writes a finished game and its per-ply analytics rows (deduped
// positions + moves) in a single transaction. It is a no-op (nil) when Postgres
// is unconfigured — local dev without lio_pg_dsn degrades to object-storage
// archival only, exactly like store. Callers run this off the hot path (the
// storeGame background goroutine), so the transaction never gates game play.
func ArchiveGame(ctx context.Context, rec GameRecord, plies []PlyRecord) error {
	_, err := archiveGame(ctx, rec, plies, false)
	return err
}

// ArchiveGameIfNew archives a game only when no row with the same
// pgn_object_key already exists, reporting whether a row was inserted. This is
// the archive backfill's idempotency key: it both skips games already recorded
// by the live seam and makes a partial backfill safe to re-run.
func ArchiveGameIfNew(ctx context.Context, rec GameRecord, plies []PlyRecord) (bool, error) {
	return archiveGame(ctx, rec, plies, true)
}

// archiveGame is the shared transactional core. When ifNew is set, the game
// insert is a no-op on a pgn_object_key conflict (returning inserted=false); a
// conflicting game contributes neither its row nor its moves.
func archiveGame(ctx context.Context, rec GameRecord, plies []PlyRecord, ifNew bool) (bool, error) {
	if Pool == nil {
		return false, nil
	}

	gameUUID, err := uuid.Parse(rec.GameID)
	if err != nil {
		return false, err
	}

	tx, err := Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }() // no-op once Commit succeeds

	q := gen.New(tx)

	params := gen.InsertGameParams{
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
	}

	var gameRef int32
	if ifNew {
		// InsertGameIfNew shares InsertGame's exact column set/order, so the two
		// generated param structs are identical and convert directly.
		gameRef, err = q.InsertGameIfNew(ctx, gen.InsertGameIfNewParams(params))
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil // already archived under this pgn_object_key
		}
	} else {
		gameRef, err = q.InsertGame(ctx, params)
	}
	if err != nil {
		return false, err
	}

	for _, p := range plies {
		positionID, err := q.UpsertPosition(ctx, gen.UpsertPositionParams{
			Hash: p.PosHash,
			Ofen: p.PosOFEN,
		})
		if err != nil {
			return false, err
		}
		if err := q.InsertMove(ctx, gen.InsertMoveParams{
			GameRef:    gameRef,
			PositionID: positionID,
			ClockMs:    p.ClockMs,
			Ply:        p.Ply,
			Mv:         p.Mv,
		}); err != nil {
			return false, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	gamesTotal.Add(1)
	return true, nil
}

// ts wraps a time.Time as a valid pgtype.Timestamptz.
func ts(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
}
