package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/dechristopher/lio/db/gen"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/variant"
	"github.com/dechristopher/lio/view"
)

// TestArchivePGNRebuild verifies the archive-page PGN rebuild (archivePGN) maps
// an archived games row to the shared game.BuildPGN correctly: the seat names
// follow room.seatArchiveName ("[BOT] <glyph> <persona>" for the engine,
// "Anonymous" for an anon human), the deploy start becomes a SetUp/FEN tag, the
// opening/matchup names come from the starting OFEN, and the result re-imports
// move-for-move. This is the from-DB rebuild the copy button copies — no
// object-store fetch.
func TestArchivePGNRebuild(t *testing.T) {
	const startOFEN = "knpp/4/4/PNKP w NCFncf - 0 1"

	og, err := game.NewOctadGame(game.OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "uid_drew", // anonymous human (uid, no account)
		Black:   "",         // the bot
		OFEN:    startOFEN,
	})
	if err != nil {
		t.Fatalf("NewOctadGame failed: %v", err)
	}
	for i := 0; i < 4; i++ {
		moves := og.Game.ValidMoves()
		if len(moves) == 0 {
			break
		}
		if err := og.Game.Move(moves[0]); err != nil {
			t.Fatalf("move %d failed: %v", i, err)
		}
	}

	persona := "pawn"
	start := time.Date(2026, 7, 22, 14, 5, 0, 0, time.UTC)
	row := gen.Game{
		StartingOfen: startOFEN,
		VariantName:  "1+0",
		VariantGroup: "blitz",
		WhiteUid:     "uid_drew",
		BlackUid:     "", // bot seat: no uid + no account
		BotPersona:   &persona,
		Outcome:      string(og.Game.Outcome()),
		Reason:       "checkmate",
		StartTs:      pgtype.Timestamptz{Time: start, Valid: true},
		EndTs:        pgtype.Timestamptz{Time: start.Add(3 * time.Minute), Valid: true},
	}

	pgn := archivePGN(row, og)

	// the bot seat carries its persona glyph + name; derive the expected string
	// from the same helpers to avoid a fragile literal glyph in the test
	wantBlack := `[Black "` + game.PGNSeatName("", "", view.BotSeatGlyph(persona), view.BotSeatLabel(persona), true) + `"]`
	for _, want := range []string{
		`[White "Anonymous"]`, wantBlack,
		`[WhiteUID "uid_drew"]`, `[BlackUID ""]`,
		`[SetUp "1"]`, `[FEN "` + startOFEN + `"]`,
		// PNKP = The Citadel (white); knpp reversed = ppnk = The Bastion (black)
		`[WhiteFormation "The Citadel"]`, `[BlackFormation "The Bastion"]`,
		`[Matchup "White Dwarf"]`,
	} {
		if !strings.Contains(pgn, want) {
			t.Errorf("rebuilt PGN missing %s:\n%s", want, pgn)
		}
	}

	// the rebuilt PGN replays from the deploy start move-for-move
	sc := octad.NewScanner(strings.NewReader(pgn + "\n\n"))
	if !sc.Scan() {
		t.Fatalf("re-scanning rebuilt PGN failed: %v", sc.Err())
	}
	replayed := sc.Next()
	if got := replayed.Positions()[0].String(); got != startOFEN {
		t.Errorf("replayed start OFEN = %q, want %q", got, startOFEN)
	}
	if got, want := len(replayed.Moves()), len(og.Game.Moves()); got != want {
		t.Errorf("replayed %d moves, want %d", got, want)
	}
}
