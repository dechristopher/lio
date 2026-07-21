package engine

import (
	"os"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/bus"
)

// TestMain brings the in-process event bus up: SearchPersona goes through the
// full Search path, whose engine-channel Publish spin-waits forever without it
// (the established bus.Up test gotcha).
func TestMain(m *testing.M) {
	bus.Up()
	os.Exit(m.Run())
}

// mateInOneOFEN has white to move and at least one mating move (e.g. Qc4#:
// the queen checks along the fourth rank and Ka2 covers every escape square).
// Every non-mating move trails the mate score by thousands of eval units, so
// it doubles as the margin-safety fixture: no persona within any sane margin
// may pick a non-mate.
const mateInOneOFEN = "k3/2Q1/K3/4 w - - 0 10"

func gameFromOFEN(t *testing.T, ofen string) *octad.Game {
	t.Helper()
	o, err := octad.OFEN(ofen)
	if err != nil {
		t.Fatalf("bad OFEN %s: %v", ofen, err)
	}
	g, err := octad.NewGame(o)
	if err != nil {
		t.Fatalf("bad game from %s: %v", ofen, err)
	}
	return g
}

// TestPersonaByKey checks every ladder key resolves to itself and that empty
// or unknown keys fall back to the full-strength Queen (the pre-persona
// behavior of every legacy room, snapshot, and archived game).
func TestPersonaByKey(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Personas {
		if seen[p.Key] {
			t.Errorf("duplicate persona key %q", p.Key)
		}
		seen[p.Key] = true
		if got := PersonaByKey(p.Key); got.Name != p.Name {
			t.Errorf("PersonaByKey(%q) = %q, want %q", p.Key, got.Name, p.Name)
		}
	}
	for _, key := range []string{"", "garbage", "QUEEN"} {
		if got := PersonaByKey(key); got.Key != "queen" {
			t.Errorf("PersonaByKey(%q) = %q, want queen fallback", key, got.Key)
		}
	}
	if !PersonaByKey("queen").fullStrength() {
		t.Error("queen persona must be full-strength (untouched search path)")
	}
	if PersonaByKey("pawn").fullStrength() {
		t.Error("pawn persona must not be full-strength")
	}
}

// TestPersonaCapDepth checks the depth ceiling handicap.
func TestPersonaCapDepth(t *testing.T) {
	cases := []struct {
		max, in, want int
	}{
		{1, 7, 1},
		{2, 2, 2},
		{6, 3, 3},
		{0, 7, 7}, // no cap
	}
	for _, c := range cases {
		p := Persona{MaxDepth: c.max}
		if got := p.capDepth(c.in); got != c.want {
			t.Errorf("capDepth(max=%d, in=%d) = %d, want %d", c.max, c.in, got, c.want)
		}
	}
}

// TestPickAmong hand-checks the top-N + margin filter for both colors: the
// pick must always come from the moves within margin of best, capped at
// maxMoves candidates, and a huge (mate-sized) gap must always be excluded.
func TestPickAmong(t *testing.T) {
	white := gameFromOFEN(t, "ppkn/4/4/NKPP w NCFncf - 0 1")
	black := gameFromOFEN(t, "ppkn/4/4/NKPP b NCFncf - 0 1")

	cases := []struct {
		name     string
		game     *octad.Game
		evals    []float64
		maxMoves int
		margin   float64
		allowed  map[float64]bool
	}{
		{
			name: "white margin excludes trailing moves",
			game: white, evals: []float64{50, 49, 10, -WinVal},
			maxMoves: 3, margin: 5,
			allowed: map[float64]bool{50: true, 49: true},
		},
		{
			name: "white maxMoves caps within-margin pool",
			game: white, evals: []float64{50, 49, 48, 47},
			maxMoves: 2, margin: 100,
			allowed: map[float64]bool{50: true, 49: true},
		},
		{
			name: "single best when maxMoves is 1",
			game: white, evals: []float64{50, 50, 10},
			maxMoves: 1, margin: 100,
			allowed: map[float64]bool{50: true},
		},
		{
			name: "black picks most-negative side",
			game: black, evals: []float64{-50, -49, 0, WinVal},
			maxMoves: 3, margin: 5,
			allowed: map[float64]bool{-50: true, -49: true},
		},
	}

	for _, c := range cases {
		for trial := 0; trial < 100; trial++ {
			results := make([]MoveEval, len(c.evals))
			for i, e := range c.evals {
				results[i] = MoveEval{Eval: e}
			}
			got := pickAmong(c.game, results, c.maxMoves, c.margin)
			if !c.allowed[got.Eval] {
				t.Fatalf("%s: picked eval %.1f outside allowed set %v", c.name, got.Eval, c.allowed)
			}
		}
	}
}

// TestPersonaNeverMissesForcedMate is the margin-safety regression: any
// persona with a zero blunder rate must always play a mating move when one
// exists — non-mate moves trail the mate score by far more than any ladder
// margin, so the variety pick can never select one.
func TestPersonaNeverMissesForcedMate(t *testing.T) {
	// sanity: the position really is mate in one at the shallowest depth
	if best := minimaxABRoot(gameFromOFEN(t, mateInOneOFEN), 2, nil); best.Eval < WinVal/2 {
		t.Fatalf("fixture broken: best eval %.1f is not a forced mate", best.Eval)
	}

	for _, key := range []string{"bishop", "rook", "queen"} {
		p := PersonaByKey(key)
		for trial := 0; trial < 10; trial++ {
			got := SearchPersona(mateInOneOFEN, nil, 4, 150*time.Millisecond, p)
			g := gameFromOFEN(t, mateInOneOFEN)
			if err := g.Move(&got.Move); err != nil {
				t.Fatalf("%s picked illegal move %s: %v", key, got.Move.String(), err)
			}
			if g.Outcome() != octad.WhiteWon {
				t.Fatalf("%s missed mate in one: played %s", key, got.Move.String())
			}
		}
	}
}

// TestPersonaBlunderIsLegal drives the blunder branch (rate 1) and checks the
// uniform random pick is always a legal move for the position.
func TestPersonaBlunderIsLegal(t *testing.T) {
	p := Persona{Key: "test-blunderer", VarietyMoves: 2, VarietyMargin: 5, BlunderRate: 1, MaxDepth: 1}
	for trial := 0; trial < 20; trial++ {
		got := SearchPersona("ppkn/4/4/NKPP w NCFncf - 0 1", nil, 7, 100*time.Millisecond, p)
		g := gameFromOFEN(t, "ppkn/4/4/NKPP w NCFncf - 0 1")
		if err := g.Move(&got.Move); err != nil {
			t.Fatalf("blunder picked illegal move %s: %v", got.Move.String(), err)
		}
	}
}
