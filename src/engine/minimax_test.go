package engine

import (
	"math/rand"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
)

// absEval mirrors the engine's leaf convention: an absolute,
// white-positive static score. Evaluate is side-to-move-relative,
// so we flip the sign when black is to move. This is exactly what
// mmABMax (returns Evaluate) and mmABMin (returns -Evaluate) do.
func absEval(g *octad.Game) float64 {
	if g.Position().Turn() == octad.White {
		return Evaluate(g)
	}
	return -Evaluate(g)
}

// refMinimax is a dead-simple, sequential, no-pruning reference
// minimax operating in absolute white-positive space. It is the
// ground truth we compare the parallel alpha-beta engine against.
func refMinimax(g *octad.Game, depth int) float64 {
	moves := g.ValidMoves()
	if depth == 0 || len(moves) == 0 {
		return absEval(g)
	}
	white := g.Position().Turn() == octad.White
	best := WinVal * 2
	if white {
		best = -best
	}
	for _, m := range moves {
		if err := g.Move(m); err != nil {
			panic(err)
		}
		v := refMinimax(g, depth-1)
		g.UndoMove()
		if white && v > best {
			best = v
		} else if !white && v < best {
			best = v
		}
	}
	return best
}

// refBest returns the best achievable root value and the set of root
// moves (as UOI strings) that achieve it, per the reference minimax.
// Mirrors the engine's depth semantics: the engine plays a root move
// then searches `depth` more plies, so each child is searched to depth.
func refBest(g *octad.Game, depth int) (float64, map[string]bool) {
	white := g.Position().Turn() == octad.White
	best := WinVal * 2
	if white {
		best = -best
	}
	vals := map[string]float64{}
	for _, m := range g.ValidMoves() {
		if err := g.Move(m); err != nil {
			panic(err)
		}
		v := refMinimax(g, depth)
		g.UndoMove()
		vals[m.String()] = v
		if white && v > best {
			best = v
		} else if !white && v < best {
			best = v
		}
	}
	bestMoves := map[string]bool{}
	for uoi, v := range vals {
		if v == best {
			bestMoves[uoi] = true
		}
	}
	return best, bestMoves
}

// randomPositions plays random legal moves to generate a spread of
// non-terminal test positions.
func randomPositions(t *testing.T, n, maxPlies int) []string {
	t.Helper()
	rng := rand.New(rand.NewSource(42))
	var ofens []string
	for len(ofens) < n {
		o, _ := octad.OFEN("ppkn/4/4/NKPP w NCFncf - 0 1")
		g, _ := octad.NewGame(o)
		plies := rng.Intn(maxPlies + 1)
		ok := true
		for i := 0; i < plies; i++ {
			moves := g.ValidMoves()
			if len(moves) == 0 || g.Outcome() != octad.NoOutcome {
				ok = false
				break
			}
			_ = g.Move(moves[rng.Intn(len(moves))])
		}
		if !ok || g.Outcome() != octad.NoOutcome || len(g.ValidMoves()) == 0 {
			continue
		}
		ofens = append(ofens, g.Position().String())
	}
	return ofens
}

const diagDepth = 3

// TestMinimaxMatchesReference checks that the parallel alpha-beta engine
// returns the true minimax value and an optimal move for many positions.
func TestMinimaxMatchesReference(t *testing.T) {
	ofens := randomPositions(t, 40, 6)
	mismatches := 0
	for _, ofen := range ofens {
		o, _ := octad.OFEN(ofen)
		g, _ := octad.NewGame(o)

		wantBest, bestMoves := refBest(g, diagDepth)

		o2, _ := octad.OFEN(ofen)
		g2, _ := octad.NewGame(o2)
		got := minimaxABRoot(g2, diagDepth, nil)

		evalOK := got.Eval == wantBest
		moveOK := bestMoves[got.Move.String()]
		if !evalOK || !moveOK {
			mismatches++
			if mismatches <= 15 {
				t.Errorf("OFEN %s\n  engine: move=%s eval=%.1f\n  ref:    best=%.1f optimalMoves=%v",
					ofen, got.Move.String(), got.Eval, wantBest, keys(bestMoves))
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("%d/%d positions mismatched reference", mismatches, len(ofens))
	}
}

// TestNegamaxMatchesReference checks that the negamax search returns the true
// minimax value (absolute, white-positive) and an optimal move. Regression
// test for negamax returning a never-updated -Inf and using a swapped window.
func TestNegamaxMatchesReference(t *testing.T) {
	ofens := randomPositions(t, 40, 6)
	mismatches := 0
	for _, ofen := range ofens {
		o, _ := octad.OFEN(ofen)
		g, _ := octad.NewGame(o)
		wantBest, bestMoves := refBest(g, diagDepth)

		o2, _ := octad.OFEN(ofen)
		g2, _ := octad.NewGame(o2)
		got := searchNegamaxAB(g2, diagDepth)

		if got.Eval != wantBest || !bestMoves[got.Move.String()] {
			mismatches++
			if mismatches <= 15 {
				t.Errorf("OFEN %s\n  negamax: move=%s eval=%.1f\n  ref:     best=%.1f optimalMoves=%v",
					ofen, got.Move.String(), got.Eval, wantBest, keys(bestMoves))
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("%d/%d positions mismatched reference", mismatches, len(ofens))
	}
}

// TestMinimaxDeterminism runs the same position many times and verifies the
// engine always returns the optimal evaluation (the move may vary among ties).
func TestMinimaxDeterminism(t *testing.T) {
	ofens := randomPositions(t, 8, 6)
	for _, ofen := range ofens {
		o, _ := octad.OFEN(ofen)
		g, _ := octad.NewGame(o)
		wantBest, bestMoves := refBest(g, diagDepth)

		evals := map[float64]int{}
		badMove := 0
		const trials = 25
		for i := 0; i < trials; i++ {
			o2, _ := octad.OFEN(ofen)
			g2, _ := octad.NewGame(o2)
			got := minimaxABRoot(g2, diagDepth, nil)
			evals[got.Eval]++
			if !bestMoves[got.Move.String()] {
				badMove++
			}
		}
		if len(evals) != 1 || badMove > 0 {
			t.Errorf("OFEN %s nondeterministic: evalDist=%v want=%.1f badMovePicks=%d/%d optimal=%v",
				ofen, evals, wantBest, badMove, trials, keys(bestMoves))
		}
	}
}

// TestDeepeningRootMatchesFixedDepth checks that iterative deepening with a
// deadline it never hits lands on the same result as the fixed-depth search:
// the final iteration is exactly minimaxABRoot's evaluateRootMoves pass.
func TestDeepeningRootMatchesFixedDepth(t *testing.T) {
	ofens := randomPositions(t, 10, 6)
	for _, ofen := range ofens {
		o, _ := octad.OFEN(ofen)
		g, _ := octad.NewGame(o)
		wantBest, bestMoves := refBest(g, diagDepth)

		o2, _ := octad.OFEN(ofen)
		g2, _ := octad.NewGame(o2)
		got, results := deepeningRoot(g2, diagDepth, time.Now().Add(time.Minute), nil)

		if got.Eval != wantBest || !bestMoves[got.Move.String()] {
			t.Errorf("OFEN %s\n  deepening: move=%s eval=%.1f\n  ref:       best=%.1f optimalMoves=%v",
				ofen, got.Move.String(), got.Eval, wantBest, keys(bestMoves))
		}
		if len(results) != len(g.ValidMoves()) {
			t.Errorf("OFEN %s: got %d root results, want %d", ofen, len(results), len(g.ValidMoves()))
		}
	}
}

// TestDeepeningRootDeadline checks that a deep search under a tight budget
// returns a legal move promptly instead of running to full depth: the deadline
// must cancel the in-flight iteration and bubble up the best completed move.
func TestDeepeningRootDeadline(t *testing.T) {
	const budget = 100 * time.Millisecond
	for _, ofen := range randomPositions(t, 5, 6) {
		o, _ := octad.OFEN(ofen)
		g, _ := octad.NewGame(o)

		start := time.Now()
		got, _ := deepeningRoot(g, 20, time.Now().Add(budget), nil)
		elapsed := time.Since(start)

		// generous slack over the budget: the abort must unwind promptly, but
		// slow CI machines shouldn't flake this
		if elapsed > budget+time.Second {
			t.Errorf("OFEN %s: search ran %s, budget was %s", ofen, elapsed, budget)
		}
		if !legalMove(g, got.Move.String()) {
			t.Errorf("OFEN %s: returned illegal move %s", ofen, got.Move.String())
		}
	}
}

// TestDeepeningRootExpiredDeadline checks the last-resort path: even with a
// deadline already in the past, the search still returns a legal move (via the
// unbounded depth-1 fallback) rather than nothing.
func TestDeepeningRootExpiredDeadline(t *testing.T) {
	for _, ofen := range randomPositions(t, 5, 6) {
		o, _ := octad.OFEN(ofen)
		g, _ := octad.NewGame(o)

		got, _ := deepeningRoot(g, 7, time.Now().Add(-time.Second), nil)
		if !legalMove(g, got.Move.String()) {
			t.Errorf("OFEN %s: returned illegal move %s", ofen, got.Move.String())
		}
	}
}

func legalMove(g *octad.Game, uoi string) bool {
	for _, m := range g.ValidMoves() {
		if m.String() == uoi {
			return true
		}
	}
	return false
}

func keys(m map[string]bool) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
