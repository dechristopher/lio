package db

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

// TestHeadToHeadVsBot exercises the all-time score-vs-persona query behind the
// timeline's historical score for bot games: the human's cumulative score, the
// bot's, and the game count, summed only over that human's games versus that
// exact persona (a different persona, and games by other users, are excluded).
func TestHeadToHeadVsBot(t *testing.T) {
	skipNoDB(t)
	ctx := context.Background()

	email := "h2hbot" + uuid.NewString()[:6] + "@example.com"
	userID, err := CreateUser("h2hbot"+time.Now().Format("150405.000"), &email, "$argon2id$fake")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	t.Cleanup(func() {
		_, _ = Pool.Exec(context.Background(),
			"DELETE FROM games WHERE white_user_id = $1 OR black_user_id = $1", userID)
		_, _ = Pool.Exec(context.Background(), "DELETE FROM users WHERE id = $1", userID)
	})

	plies, blob, startOFEN := buildGamePlies(t, 4)

	// archiveBotGame inserts one finished, room-less bot game: the human seat
	// (humanWhite decides its color) carries userID; the bot seat is uid-less and
	// account-less with the given persona stamp.
	archiveBotGame := func(humanWhite bool, persona, outcome string) {
		rec := GameRecord{
			GameID: uuid.NewString(), StartTs: time.Now(), EndTs: time.Now(),
			VariantName: "Test", VariantGroup: "blitz", Casual: true,
			Outcome: outcome, Method: 1, Reason: "checkmate",
			StartingOFEN: startOFEN, Moves: blob,
			PGNObjectKey: "test/h2hbot-" + uuid.NewString() + ".pgn",
			BotPersona:   persona,
		}
		if humanWhite {
			rec.WhiteUID, rec.WhiteUserID = "u_human", &userID
			rec.BlackUID = "" // bot
		} else {
			rec.BlackUID, rec.BlackUserID = "u_human", &userID
			rec.WhiteUID = "" // bot
		}
		if _, err := ArchiveGame(ctx, rec, plies); err != nil {
			t.Fatalf("archive bot game: %v", err)
		}
	}

	// vs Pawn: human wins twice (as white 1-0, as black 0-1), loses once, draws once
	archiveBotGame(true, "pawn", "1-0")     // human (white) wins  → user +1
	archiveBotGame(false, "pawn", "0-1")    // human (black) wins  → user +1
	archiveBotGame(true, "pawn", "0-1")     // bot (black) wins    → bot  +1
	archiveBotGame(true, "pawn", "1/2-1/2") // draw                → +0.5 each
	// a different persona must not bleed into the Pawn tally
	archiveBotGame(true, "knight", "1-0")

	h := HeadToHeadVsBot(&userID, "pawn")
	if h.Games != 4 {
		t.Errorf("games = %d, want 4", h.Games)
	}
	if h.UserScore != 2.5 {
		t.Errorf("user score = %v, want 2.5", h.UserScore)
	}
	if h.BotScore != 1.5 {
		t.Errorf("bot score = %v, want 1.5", h.BotScore)
	}

	// the excluded persona tallies on its own
	if k := HeadToHeadVsBot(&userID, "knight"); k.Games != 1 || k.UserScore != 1 {
		t.Errorf("knight tally = %+v, want games=1 user=1", k)
	}

	// guards: nil user, empty persona, and a persona with no games all zero out
	if z := HeadToHeadVsBot(nil, "pawn"); z.Games != 0 {
		t.Errorf("nil user should be zero, got %+v", z)
	}
	if z := HeadToHeadVsBot(&userID, ""); z.Games != 0 {
		t.Errorf("empty persona should be zero, got %+v", z)
	}
	if z := HeadToHeadVsBot(&userID, "bishop"); z.Games != 0 {
		t.Errorf("unplayed persona should be zero, got %+v", z)
	}
}
