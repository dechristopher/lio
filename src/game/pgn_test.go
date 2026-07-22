package game

import (
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/variant"
)

// newTestGame builds a short game from the given (possibly empty) start OFEN and
// plays n legal moves, for exercising BuildPGN.
func newTestGame(t *testing.T, ofen string, n int) *OctadGame {
	t.Helper()
	g, err := NewOctadGame(OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "white",
		Black:   "black",
		OFEN:    ofen,
	})
	if err != nil {
		t.Fatalf("NewOctadGame(%q) failed: %v", ofen, err)
	}
	for i := 0; i < n; i++ {
		moves := g.Game.ValidMoves()
		if len(moves) == 0 {
			break
		}
		if err := g.Game.Move(moves[0]); err != nil {
			t.Fatalf("playing move %s failed: %v", moves[0], err)
		}
	}
	return g
}

func sampleMeta() PGNMeta {
	return PGNMeta{
		Event:          "Lioctad Test Match",
		Site:           "https://lioctad.org",
		Variant:        "1+0",
		Group:          "blitz",
		White:          "drewtest",
		Black:          "BOT",
		WhiteUID:       "uid_drew",
		BlackUID:       "",
		Result:         "1-0",
		Reason:         "checkmate",
		Start:          time.Date(2026, 7, 22, 9, 0, 0, 0, time.UTC),
		End:            time.Date(2026, 7, 22, 9, 3, 30, 0, time.UTC),
		StartOFEN:      "ppkn/4/4/NKPP w NCFncf - 0 1",
		WhiteFormation: "The Standard",
		BlackFormation: "The Standard",
		Matchup:        "Standing Wave",
	}
}

// TestBuildPGNDeterministic verifies BuildPGN is a pure function of its inputs:
// the same meta + game + timing produce byte-identical output. This is the
// guarantee that lets the live archival path and the archive-page rebuild agree.
func TestBuildPGNDeterministic(t *testing.T) {
	g := newTestGame(t, "", 4)
	m := sampleMeta()
	a := BuildPGN(m, &g.Game, g.MoveTimes)
	b := BuildPGN(m, &g.Game, g.MoveTimes)
	if a != b {
		t.Fatalf("BuildPGN is not deterministic:\n--- a ---\n%s\n--- b ---\n%s", a, b)
	}
}

// TestBuildPGNNameTags verifies the opening/matchup names are emitted as tag
// pairs (and omitted when absent).
func TestBuildPGNNameTags(t *testing.T) {
	g := newTestGame(t, "", 2)
	pgn := BuildPGN(sampleMeta(), &g.Game, g.MoveTimes)
	for _, want := range []string{
		`[WhiteFormation "The Standard"]`,
		`[BlackFormation "The Standard"]`,
		`[Matchup "Standing Wave"]`,
	} {
		if !strings.Contains(pgn, want) {
			t.Errorf("PGN missing %s:\n%s", want, pgn)
		}
	}

	// an empty WhiteFormation omits all three name tags
	m := sampleMeta()
	m.WhiteFormation, m.BlackFormation, m.Matchup = "", "", ""
	bare := BuildPGN(m, &g.Game, g.MoveTimes)
	if strings.Contains(bare, "Formation") || strings.Contains(bare, "[Matchup ") {
		t.Errorf("PGN should omit name tags when unresolved:\n%s", bare)
	}
}

// TestBuildPGNClockAndReimport verifies %clk comments are emitted per move when
// timing is present, and the annotated PGN still re-imports move-for-move
// (octad's decoder strips comments).
func TestBuildPGNClockAndReimport(t *testing.T) {
	g := newTestGame(t, "", 0)
	for i := 0; i < 3; i++ {
		moves := g.Game.ValidMoves()
		if err := g.Game.Move(moves[0]); err != nil {
			t.Fatalf("move %d failed: %v", i, err)
		}
		g.MoveTimes = append(g.MoveTimes, MoveTime{ThinkMs: int64(i) * 500, ClockMs: 60000 - int64(i)*500})
	}
	pgn := BuildPGN(sampleMeta(), &g.Game, g.MoveTimes)

	if got, want := strings.Count(pgn, "[%clk "), 3; got != want {
		t.Fatalf("PGN carries %d %%clk comments, want %d:\n%s", got, want, pgn)
	}

	sc := octad.NewScanner(strings.NewReader(pgn + "\n\n"))
	if !sc.Scan() {
		t.Fatalf("re-scanning PGN failed: %v", sc.Err())
	}
	if got, want := len(sc.Next().Moves()), 3; got != want {
		t.Errorf("re-imported PGN has %d moves, want %d", got, want)
	}
}
