package db

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/google/uuid"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/game"
)

// These tests exercise the real archive path against a live Postgres. They skip
// when DEV_LIO_PG_DSN is unset, matching the repo's degrade-when-infra-absent
// convention (bring one up with dev/dev.sh up).

func skipNoDB(t *testing.T) {
	t.Helper()
	if os.Getenv("DEV_LIO_PG_DSN") == "" {
		t.Skip("DEV_LIO_PG_DSN unset; skipping postgres integration test")
	}
	if Pool == nil {
		Up() // reads DEV_LIO_PG_DSN via config.ReadSecretFallback in local env
	}
	if Pool == nil {
		t.Skip("postgres unreachable; skipping")
	}
}

// buildGamePlies plays plyCount deterministic (first-legal) moves from the start
// and returns the analytics plies, the packed move blob, and the starting OFEN.
func buildGamePlies(t *testing.T, plyCount int) (plies []PlyRecord, blob []byte, startOFEN string) {
	t.Helper()
	g, err := octad.NewGame()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < plyCount; i++ {
		vm := g.ValidMoves()
		if len(vm) == 0 {
			break
		}
		if err := g.Move(vm[0]); err != nil {
			t.Fatal(err)
		}
	}
	moves := g.Moves()
	positions := g.Positions()
	blob = make([]byte, 0, len(moves)*2)
	for i, m := range moves {
		packed := game.PackMove(m)
		blob = append(blob, byte(packed>>8), byte(packed))
		pos := positions[i+1]
		h := pos.Hash()
		plies = append(plies, PlyRecord{
			Ply:     int16(i + 1),
			Mv:      packed,
			PosHash: h[:],
			PosOFEN: pos.String(),
		})
	}
	return plies, blob, positions[0].String()
}

func countPositions(t *testing.T, ctx context.Context) int64 {
	t.Helper()
	var n int64
	if err := Pool.QueryRow(ctx, "SELECT count(*) FROM positions").Scan(&n); err != nil {
		t.Fatalf("count positions: %v", err)
	}
	return n
}

func TestArchiveGameIfNewIdempotent(t *testing.T) {
	skipNoDB(t)
	ctx := context.Background()

	plies, blob, startOFEN := buildGamePlies(t, 4)
	key := "backfill-test/" + uuid.NewString() + ".pgn"
	rec := GameRecord{
		RoomID: "", Creator: "", RaceTo: 0, WhiteScore: 0, BlackScore: 0,
		Reason: "checkmate", GameID: uuid.NewString(),
		StartTs: time.Now(), EndTs: time.Now(),
		WhiteUID: "bf_w", BlackUID: "bf_b",
		VariantName: "Test", VariantGroup: "blitz", Casual: false,
		Outcome: "1-0", Method: 1, StartingOFEN: startOFEN,
		Moves: blob, PGNObjectKey: key,
	}
	t.Cleanup(func() {
		_, _ = Pool.Exec(context.Background(), "DELETE FROM games WHERE pgn_object_key = $1", key)
	})

	// first archive inserts
	inserted, err := ArchiveGameIfNew(ctx, rec, plies)
	if err != nil {
		t.Fatalf("first archive: %v", err)
	}
	if !inserted {
		t.Fatal("first ArchiveGameIfNew should have inserted")
	}

	// a second archive with the SAME pgn_object_key (even a different game_id,
	// as a live-archived game would have) is skipped, not duplicated
	rec2 := rec
	rec2.GameID = uuid.NewString()
	inserted, err = ArchiveGameIfNew(ctx, rec2, plies)
	if err != nil {
		t.Fatalf("second archive: %v", err)
	}
	if inserted {
		t.Error("second ArchiveGameIfNew with same pgn_object_key should have been skipped")
	}

	var n int
	if err := Pool.QueryRow(ctx, "SELECT count(*) FROM games WHERE pgn_object_key = $1", key).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Errorf("expected exactly 1 row for pgn_object_key, got %d", n)
	}
}

func TestArchiveGameRoundTrip(t *testing.T) {
	skipNoDB(t)
	ctx := context.Background()

	plies, blob, startOFEN := buildGamePlies(t, 6)

	rec := GameRecord{
		RoomID: "testroom", Creator: "u_white", RaceTo: 0,
		WhiteScore: 1, BlackScore: 0, Reason: "checkmate",
		GameID: uuid.NewString(), StartTs: time.Now(), EndTs: time.Now(),
		WhiteUID: "u_white", BlackUID: "u_black",
		VariantName: "Test", VariantGroup: "blitz", Casual: false,
		Outcome: "1-0", Method: 1, StartingOFEN: startOFEN,
		Moves: blob, PGNObjectKey: "test/key.pgn",
	}
	t.Cleanup(func() {
		_, _ = Pool.Exec(context.Background(),
			"DELETE FROM games WHERE room_id IN ('testroom','testroom2')")
		_, _ = Pool.Exec(context.Background(),
			"DELETE FROM rooms WHERE room_id IN ('testroom','testroom2')")
	})

	gamesBefore := TotalGames()
	if err := ArchiveGame(ctx, rec, plies); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if got := TotalGames(); got != gamesBefore+1 {
		t.Errorf("games counter: got %d want %d after one archive", got, gamesBefore+1)
	}

	q := gen.New(Pool)

	// every distinct position of the game must now exist (deduped by hash)
	for _, p := range plies {
		if _, err := q.GetPositionByHash(ctx, p.PosHash); err != nil {
			t.Fatalf("position %x missing after archive: %v", p.PosHash, err)
		}
	}

	// game row + move rows land, move count == ply count
	gameUUID, _ := uuid.Parse(rec.GameID)
	gm, err := q.GetGameByUUID(ctx, gameUUID)
	if err != nil {
		t.Fatalf("get game: %v", err)
	}
	if gm.Outcome != "1-0" || gm.RoomID != "testroom" {
		t.Errorf("unexpected game row: outcome=%q room=%q", gm.Outcome, gm.RoomID)
	}
	mvs, err := q.ListGameMoves(ctx, gm.ID)
	if err != nil {
		t.Fatalf("list moves: %v", err)
	}
	if len(mvs) != len(plies) {
		t.Fatalf("move count: got %d want %d", len(mvs), len(plies))
	}

	// dedup: archiving an identical game (new uuid) inserts zero new positions
	before := countPositions(t, ctx)
	rec2 := rec
	rec2.GameID = uuid.NewString()
	rec2.RoomID = "testroom2"
	rec2.PGNObjectKey = "test/key2.pgn" // distinct key (pgn_object_key is now UNIQUE)
	if err := ArchiveGame(ctx, rec2, plies); err != nil {
		t.Fatalf("archive dup: %v", err)
	}
	if after := countPositions(t, ctx); after != before {
		t.Errorf("dedup failed: positions grew %d→%d on an identical game", before, after)
	}
	if got := TotalGames(); got != gamesBefore+2 {
		t.Errorf("games counter: got %d want %d after two archives", got, gamesBefore+2)
	}

	// both games surface in the white player's history
	pg, err := q.ListPlayerGames(ctx, gen.ListPlayerGamesParams{
		WhiteUid: "u_white", Limit: 10, Offset: 0,
	})
	if err != nil {
		t.Fatalf("list player games: %v", err)
	}
	if len(pg) < 2 {
		t.Errorf("expected >=2 player games, got %d", len(pg))
	}
}
