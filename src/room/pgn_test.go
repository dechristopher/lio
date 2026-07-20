package room

import (
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/variant"
)

// TestBuildArchivePGNDeployStart verifies that a game beginning from a
// non-standard (deploy-mode) home rank records the starting OFEN as a SetUp/FEN
// tag pair, and that the resulting PGN replays from that exact position rather
// than the standard octad start.
func TestBuildArchivePGNDeployStart(t *testing.T) {
	// a rearranged white home rank (P,N,K,P) — the sort of position the blind
	// deploy phase assembles; distinct from the standard ppkn/4/4/NKPP start
	const deployOFEN = "knpp/4/4/PNKP w NCFncf - 0 1"

	g, err := game.NewOctadGame(game.OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "white",
		Black:   "black",
		OFEN:    deployOFEN,
	})
	if err != nil {
		t.Fatalf("NewOctadGame(deploy) failed: %v", err)
	}

	// play a few real moves so the PGN carries movetext to replay
	for i := 0; i < 4; i++ {
		moves := g.Game.ValidMoves()
		if len(moves) == 0 {
			break
		}
		if err := g.Game.Move(moves[0]); err != nil {
			t.Fatalf("playing move %s failed: %v", moves[0], err)
		}
	}

	pgn := buildArchivePGN(*g, db.GameRecord{}, time.Now())

	if !strings.Contains(pgn, `[SetUp "1"]`) {
		t.Errorf("deploy PGN missing SetUp tag:\n%s", pgn)
	}
	if want := `[FEN "` + deployOFEN + `"]`; !strings.Contains(pgn, want) {
		t.Errorf("deploy PGN missing %s:\n%s", want, pgn)
	}

	// the tagged FEN must actually drive reconstruction: re-importing the PGN has
	// to reproduce the deployed starting position, not the standard start
	sc := octad.NewScanner(strings.NewReader(pgn + "\n\n"))
	if !sc.Scan() {
		t.Fatalf("re-scanning archived PGN failed: %v", sc.Err())
	}
	replayed := sc.Next()
	if got := replayed.Positions()[0].String(); got != deployOFEN {
		t.Errorf("replayed start OFEN = %q, want deployed %q", got, deployOFEN)
	}
}

// TestBuildArchivePGNStandardStart verifies that a game from the standard octad
// start omits the SetUp/FEN tags — they'd be redundant and change existing
// output for normal games.
func TestBuildArchivePGNStandardStart(t *testing.T) {
	g, err := game.NewOctadGame(game.OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "white",
		Black:   "black",
	})
	if err != nil {
		t.Fatalf("NewOctadGame(standard) failed: %v", err)
	}

	end := time.Date(2026, 7, 14, 9, 35, 0, 0, time.UTC)
	pgn := buildArchivePGN(*g, db.GameRecord{}, end)

	if strings.Contains(pgn, "[FEN ") || strings.Contains(pgn, "[SetUp ") {
		t.Errorf("standard-start PGN should carry no SetUp/FEN tag:\n%s", pgn)
	}

	// the finish time is recorded so a later archive backfill recovers end_ts
	if !strings.Contains(pgn, `[EndDate "2026.07.14"]`) || !strings.Contains(pgn, `[EndTime "09:35:00"]`) {
		t.Errorf("standard-start PGN missing EndDate/EndTime tags:\n%s", pgn)
	}
}

// TestBuildArchivePGNSeatTags verifies the Phase-2 PGN seat tags: the standard
// White/Black tags carry the display names (username / "BOT"), while the raw
// session uids move to the dedicated WhiteUID/BlackUID tags. The split keeps
// the PGN human-readable while the backfill still recovers machine identity
// (see backfill.seatUID). An empty GameRecord falls back to the game's uids in
// White/Black (covered by the other tests).
func TestBuildArchivePGNSeatTags(t *testing.T) {
	g, err := game.NewOctadGame(game.OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "uid_drew", // white is a logged-in human
		Black:   "",         // black is the bot (empty uid)
	})
	if err != nil {
		t.Fatalf("NewOctadGame failed: %v", err)
	}
	rec := db.GameRecord{WhiteName: "drewtest", BlackName: "BOT"}
	pgn := buildArchivePGN(*g, rec, time.Now())

	for _, want := range []string{
		`[White "drewtest"]`, `[Black "BOT"]`,
		`[WhiteUID "uid_drew"]`, `[BlackUID ""]`,
	} {
		if !strings.Contains(pgn, want) {
			t.Errorf("PGN missing %s:\n%s", want, pgn)
		}
	}
	// the display name must not leak into the uid tag position
	if strings.Contains(pgn, `[WhiteUID "drewtest"]`) {
		t.Errorf("display name leaked into WhiteUID tag:\n%s", pgn)
	}
}

// TestBuildArchivePGNClockComments verifies that a game with recorded per-ply
// timing emits one %clk comment per move (the mover's remaining clock) and
// that the annotated PGN still re-imports move-for-move — octad's decoder
// strips comments, which is what keeps the backfill replay path safe.
func TestBuildArchivePGNClockComments(t *testing.T) {
	g, err := game.NewOctadGame(game.OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "white",
		Black:   "black",
	})
	if err != nil {
		t.Fatalf("NewOctadGame failed: %v", err)
	}

	for i := 0; i < 3; i++ {
		moves := g.Game.ValidMoves()
		if err := g.Game.Move(moves[0]); err != nil {
			t.Fatalf("playing move %s failed: %v", moves[0], err)
		}
		g.MoveTimes = append(g.MoveTimes, game.MoveTime{
			ThinkMs: int64(i) * 750,
			ClockMs: 15000 - int64(i)*750,
		})
	}

	pgn := buildArchivePGN(*g, db.GameRecord{}, time.Now())

	if got, want := strings.Count(pgn, "[%clk "), 3; got != want {
		t.Fatalf("PGN carries %d %%clk comments, want %d:\n%s", got, want, pgn)
	}
	// spot-check the formatting of the first two: full budget, then -750ms
	if !strings.Contains(pgn, "{ [%clk 0:00:15.00] }") ||
		!strings.Contains(pgn, "{ [%clk 0:00:14.25] }") {
		t.Errorf("PGN missing expected %%clk values:\n%s", pgn)
	}

	sc := octad.NewScanner(strings.NewReader(pgn + "\n\n"))
	if !sc.Scan() {
		t.Fatalf("re-scanning annotated PGN failed: %v", sc.Err())
	}
	if got, want := len(sc.Next().Moves()), 3; got != want {
		t.Errorf("annotated PGN replayed %d moves, want %d", got, want)
	}
}
