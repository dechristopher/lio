package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/dechristopher/lio/db/gen"
)

// TestArchiveRoomsAndOrdinals exercises the permalink plumbing the archive
// transaction maintains: per-room game ordinals (game_index), the upserted
// rooms row, the close marker, and the read wrappers the archive pages use.
func TestArchiveRoomsAndOrdinals(t *testing.T) {
	skipNoDB(t)
	ctx := context.Background()

	roomID := "tstRm" + uuid.NewString()[:2] // unique-ish 7-char style id
	t.Cleanup(func() {
		_, _ = Pool.Exec(context.Background(), "DELETE FROM games WHERE room_id = $1", roomID)
		_, _ = Pool.Exec(context.Background(), "DELETE FROM rooms WHERE room_id = $1", roomID)
	})

	plies, blob, startOFEN := buildGamePlies(t, 4)
	rec := GameRecord{
		RoomID: roomID, Creator: "u_creator", RaceTo: 3,
		WhiteScore: 1, BlackScore: 0, Reason: "checkmate",
		GameID: uuid.NewString(), StartTs: time.Now(), EndTs: time.Now(),
		WhiteUID: "u_white", BlackUID: "u_black",
		VariantName: "Test", VariantGroup: "blitz", Casual: false,
		Outcome: "1-0", Method: 1, StartingOFEN: startOFEN,
		Moves: blob, PGNObjectKey: "test/rooms-" + rec1Key(),
	}

	if exists := RoomIDExists(roomID); exists {
		t.Fatalf("room %s should not exist yet", roomID)
	}

	if err := ArchiveGame(ctx, rec, plies); err != nil {
		t.Fatalf("archive game 1: %v", err)
	}

	// game 2: seats swapped, score progressed
	rec2 := rec
	rec2.GameID = uuid.NewString()
	rec2.WhiteUID, rec2.BlackUID = "u_black", "u_white"
	rec2.WhiteScore, rec2.BlackScore = 1, 1
	rec2.Outcome = "0-1"
	rec2.PGNObjectKey = "test/rooms-" + rec1Key()
	if err := ArchiveGame(ctx, rec2, plies); err != nil {
		t.Fatalf("archive game 2: %v", err)
	}

	// ordinals assigned 1, 2 in archive order
	games, err := ListRoomGames(roomID)
	if err != nil {
		t.Fatalf("list room games: %v", err)
	}
	if len(games) != 2 {
		t.Fatalf("expected 2 games, got %d", len(games))
	}
	for i, g := range games {
		if int(g.GameIndex) != i+1 {
			t.Errorf("game %d: game_index=%d want %d", i, g.GameIndex, i+1)
		}
	}

	// rooms row upserted and current as of the latest game
	room, found, err := GetArchivedRoom(roomID)
	if err != nil || !found {
		t.Fatalf("get archived room: found=%v err=%v", found, err)
	}
	if room.GameCount != 2 {
		t.Errorf("game_count=%d want 2", room.GameCount)
	}
	if room.WhiteUid != "u_black" || room.BlackUid != "u_white" {
		t.Errorf("seats not refreshed: white=%q black=%q", room.WhiteUid, room.BlackUid)
	}
	if room.WhiteScore != 1 || room.BlackScore != 1 {
		t.Errorf("scores not refreshed: %v-%v", room.WhiteScore, room.BlackScore)
	}
	if room.RaceTo != 3 || room.CreatorUid != "u_creator" {
		t.Errorf("insert-only fields wrong: race_to=%d creator=%q", room.RaceTo, room.CreatorUid)
	}
	if room.ClosedAt.Valid {
		t.Error("closed_at should be NULL while the room is open")
	}

	// the all-time uniqueness check the create re-roll uses
	if !RoomIDExists(roomID) {
		t.Error("RoomIDExists should be true after first archive")
	}

	// wrappers used by the permalink handlers
	g2, found, err := GetGameByUUID(rec2.GameID)
	if err != nil || !found {
		t.Fatalf("get game by uuid: found=%v err=%v", found, err)
	}
	if g2.GameIndex != 2 || g2.RoomID != roomID {
		t.Errorf("uuid lookup: game_index=%d room=%q", g2.GameIndex, g2.RoomID)
	}
	q := gen.New(Pool)
	byIdx, err := q.GetRoomGameByIndex(ctx, gen.GetRoomGameByIndexParams{
		RoomID: roomID, GameIndex: 1,
	})
	if err != nil {
		t.Fatalf("get by index: %v", err)
	}
	if byIdx.GameID.String() != rec.GameID {
		t.Error("get by index returned the wrong game")
	}

	// cosmetic close marker
	MarkRoomClosed(roomID)
	room, _, err = GetArchivedRoom(roomID)
	if err != nil {
		t.Fatalf("get after close: %v", err)
	}
	if !room.ClosedAt.Valid {
		t.Error("closed_at should be set after MarkRoomClosed")
	}
}

// rec1Key returns a unique object-store key suffix per call.
func rec1Key() string {
	return uuid.NewString() + ".pgn"
}
