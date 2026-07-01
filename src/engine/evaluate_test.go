package engine

import (
	"math/rand"
	"testing"

	"github.com/dechristopher/octad/v2"
)

// TestCheckTermSign guards the side-to-move-relative convention of Evaluate:
// being in check must lower the score of the player on the move by exactly
// CheckPenalty, regardless of color. Regression test for the inverted
// check-term sign that made the engine avoid giving check. It relies on
// staticEval being the exact non-check portion of Evaluate.
func TestCheckTermSign(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	checkedW, checkedB, notChecked := 0, 0, 0
	for iter := 0; iter < 300000 && (checkedW < 25 || checkedB < 25 || notChecked < 50); iter++ {
		o, _ := octad.OFEN("ppkn/4/4/NKPP w NCFncf - 0 1")
		g, _ := octad.NewGame(o)
		for i := 0; i < rng.Intn(12); i++ {
			moves := g.ValidMoves()
			if len(moves) == 0 || g.Outcome() != octad.NoOutcome {
				break
			}
			_ = g.Move(moves[rng.Intn(len(moves))])
		}
		if g.Outcome() != octad.NoOutcome {
			continue
		}

		want := staticEval(g, g.Position().Turn())
		if g.Position().InCheck() {
			want -= CheckPenalty
		}
		if got := Evaluate(g); got != want {
			t.Fatalf("Evaluate=%.1f want=%.1f (inCheck=%v turn=%s OFEN %s)",
				got, want, g.Position().InCheck(), g.Position().Turn(), g.Position().String())
		}

		if !g.Position().InCheck() {
			notChecked++
		} else if g.Position().Turn() == octad.White {
			checkedW++
		} else {
			checkedB++
		}
	}
	if checkedW == 0 || checkedB == 0 {
		t.Fatalf("did not sample both colors in check (white=%d black=%d)", checkedW, checkedB)
	}
}
