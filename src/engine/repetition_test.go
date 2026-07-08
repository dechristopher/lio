package engine

import (
	"strings"
	"testing"

	"github.com/dechristopher/octad/v2"
)

func TestRepetitionKey(t *testing.T) {
	cases := []struct {
		a, b string
		same bool
	}{
		// same position, different clocks: identical for repetition purposes
		{"3k/q3/4/2KN b - - 5 9", "3k/q3/4/2KN b - - 11 12", true},
		// different side to move: distinct positions
		{"3k/q3/4/2KN b - - 5 9", "3k/q3/4/2KN w - - 5 9", false},
		// different castling rights: distinct positions
		{"ppkn/4/4/NKPP w NCFncf - 0 1", "ppkn/4/4/NKPP w NCF - 0 1", false},
	}
	for _, c := range cases {
		if got := repetitionKey(c.a) == repetitionKey(c.b); got != c.same {
			t.Errorf("repetitionKey(%q) == repetitionKey(%q): got %t, want %t",
				c.a, c.b, got, c.same)
		}
	}
}

// stuckPGN is a real bot game (white) that shuffled a won KNQ-vs-K endgame
// into a threefold-repetition draw, truncated just before the drawing
// 11. Qa3+: white to move, up queen and knight against a bare king, with the
// post-Qa3+ position already on the board twice (after 7. Qxa3+ and 9. Qa3+).
const stuckPGN = `[Event "repetition regression"]
[Result "*"]

1. Kb2 a3+ 2. Ka2 Kc3 3. c2 Nxc2 4. dxc2 b3+ 5. cxb3 Kd2 6. b4=Q+ Kd3 7. Qxa3+ Kd2 8. Qb2+ Kd3 9. Qa3+ Kd2 10. Qb2+ Kd3 *`

// replayStuckGame reconstructs the shuffle game with its full position history.
func replayStuckGame(t *testing.T) *octad.Game {
	t.Helper()
	sc := octad.NewScanner(strings.NewReader(stuckPGN + "\n\n"))
	if !sc.Scan() {
		t.Fatalf("scanning stuck-game PGN failed: %v", sc.Err())
	}
	g := sc.Next()
	if g.Outcome() != octad.NoOutcome {
		t.Fatalf("stuck game should still be live, got outcome %s (%s)",
			g.Outcome(), g.Method())
	}
	return g
}

// positionOFENs returns every position the game has passed through, oldest
// first, including the current one — the engine-side analogue of
// game.OctadGame.OFENHistory.
func positionOFENs(g *octad.Game) []string {
	positions := g.Positions()
	ofens := make([]string, 0, len(positions))
	for _, p := range positions {
		ofens = append(ofens, p.String())
	}
	return ofens
}

// playEngineMove searches the position with the game's own history and plays
// the chosen move, returning its evaluation. Like the production path (Search
// gets a bare OFEN plus history strings), the search runs on a freshly built
// game: the parallel root fan-out value-copies the game and relies on its
// slices reallocating on first append, which a history-laden replayed game
// (spare slice capacity) does not guarantee.
func playEngineMove(t *testing.T, g *octad.Game, depth int) MoveEval {
	t.Helper()
	hist := RepetitionHistory(positionOFENs(g))
	o, err := octad.OFEN(g.Position().String())
	if err != nil {
		t.Fatalf("bad position OFEN %q: %v", g.Position().String(), err)
	}
	fresh, err := octad.NewGame(o)
	if err != nil {
		t.Fatalf("NewGame(%q): %v", g.Position().String(), err)
	}
	best := minimaxABRoot(fresh, depth, hist)
	for _, m := range g.ValidMoves() {
		if m.String() == best.Move.String() {
			if err := g.Move(m); err != nil {
				t.Fatalf("playing %s failed: %v", m, err)
			}
			return best
		}
	}
	t.Fatalf("engine chose illegal move %s in %s", best.Move.String(), g.Position().String())
	return best
}

// TestRepetitionAvoidance is the direct regression for the drawn game: with
// the game history supplied, the winning side must not walk into the
// automatic threefold (11. Qa3+ here) and must still know it is winning.
func TestRepetitionAvoidance(t *testing.T) {
	g := replayStuckGame(t)

	best := playEngineMove(t, g, 4)

	if g.Method() == octad.ThreefoldRepetition {
		t.Fatalf("engine repeated into the threefold draw with move %s", best.Move.String())
	}
	if best.Eval < 50 {
		t.Errorf("winning side eval collapsed to %.1f (want clearly winning, > 50)", best.Eval)
	}
}

// TestEndgameConversion plays the stuck game out with the engine on both
// sides (history-aware) and requires white to actually convert the KNQ-vs-K
// win instead of reaching any draw. This exercises the repetition scoring and
// the mop-up gradient together.
func TestEndgameConversion(t *testing.T) {
	g := replayStuckGame(t)

	const maxPlies = 40
	for ply := 0; ply < maxPlies && g.Outcome() == octad.NoOutcome; ply++ {
		playEngineMove(t, g, 4)
	}

	if g.Outcome() != octad.WhiteWon {
		t.Fatalf("engine failed to convert KNQ vs K: outcome %s (%s) after game:\n%s",
			g.Outcome(), g.Method(), g.String())
	}
}
