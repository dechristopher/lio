package engine

import (
	"reflect"
	"sort"
	"testing"

	"github.com/dechristopher/octad"
)

func squaresFromOFEN(t *testing.T, ofen string) map[octad.Square]octad.Piece {
	t.Helper()
	o, err := octad.OFEN(ofen)
	if err != nil {
		t.Fatalf("bad ofen %q: %v", ofen, err)
	}
	g, err := octad.NewGame(o)
	if err != nil {
		t.Fatalf("NewGame(%q): %v", ofen, err)
	}
	return g.Position().Board().SquareMap()
}

func sortedSquares(set map[octad.Square]bool) []string {
	out := make([]string, 0, len(set))
	for sq := range set {
		out = append(out, sq.String())
	}
	sort.Strings(out)
	return out
}

// knightOFEN: white knight b2, white king a1, black king d4. A knight on b2
// attacks a4, c4, d3, d1; the king on a1 attacks a2, b1, and (defending) b2.
const knightOFEN = "3k/4/1N2/K3 w - - 0 1"

func TestAttackedSquares(t *testing.T) {
	squares := squaresFromOFEN(t, knightOFEN)

	got := sortedSquares(attackedSquares(squares, octad.White))
	want := []string{"a2", "a4", "b1", "b2", "c4", "d1", "d3"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("white attacks = %v, want %v", got, want)
	}

	// black has only its king on d4, attacking c4, c3, d3
	gotB := sortedSquares(attackedSquares(squares, octad.Black))
	wantB := []string{"c3", "c4", "d3"}
	if !reflect.DeepEqual(gotB, wantB) {
		t.Errorf("black attacks = %v, want %v", gotB, wantB)
	}
}

func TestDefendedCount(t *testing.T) {
	squares := squaresFromOFEN(t, knightOFEN)
	whiteAtt := attackedSquares(squares, octad.White)
	blackAtt := attackedSquares(squares, octad.Black)

	// the white knight on b2 is defended by the king; the king itself is not counted
	if n := defendedCount(squares, octad.White, whiteAtt); n != 1 {
		t.Errorf("white defended = %d, want 1", n)
	}
	if n := defendedCount(squares, octad.Black, blackAtt); n != 0 {
		t.Errorf("black defended = %d, want 0", n)
	}
}

func TestKingEscapes(t *testing.T) {
	squares := squaresFromOFEN(t, knightOFEN)
	whiteAtt := attackedSquares(squares, octad.White)
	blackAtt := attackedSquares(squares, octad.Black)

	// white king a1: neighbors a2, b1 (safe) and b2 (own knight, excluded) -> 2
	if n := kingEscapes(squares, octad.White, blackAtt); n != 2 {
		t.Errorf("white king escapes = %d, want 2", n)
	}
	// black king d4: neighbors c4 (attacked), d3 (attacked), c3 (safe) -> 1
	if n := kingEscapes(squares, octad.Black, whiteAtt); n != 1 {
		t.Errorf("black king escapes = %d, want 1", n)
	}
}

func TestPawnStructureScore(t *testing.T) {
	// white pawns doubled & isolated on the c-file (c2, c3), both blocked from
	// passing by the black b4 pawn; black pawns a3 (passed) and b4 (not passed).
	squares := squaresFromOFEN(t, "1p1k/p1P1/2P1/K3 w - - 0 1")

	// white: 2 pawns each doubled (-4) and isolated (-5), neither passed = -18
	if s := pawnStructureScore(squares, octad.White); s != -2*(DoubledPawnPenalty+IsolatedPawnPenalty) {
		t.Errorf("white pawn structure = %.1f, want %.1f", s, -2*(DoubledPawnPenalty+IsolatedPawnPenalty))
	}
	// black: a3 passed (+6), b4 neither doubled/isolated/passed = +6
	if s := pawnStructureScore(squares, octad.Black); s != PassedPawnBonus {
		t.Errorf("black pawn structure = %.1f, want %.1f", s, PassedPawnBonus)
	}
}

// TestPositionalFeaturesAntisymmetric verifies the features are a pure
// differential: evaluating from white's perspective is the exact negation of
// evaluating the same board from black's perspective.
func TestPositionalFeaturesAntisymmetric(t *testing.T) {
	for _, ofen := range randomPositions(t, 60, 8) {
		squares := squaresFromOFEN(t, ofen)
		w := positionalFeatures(squares, octad.White)
		b := positionalFeatures(squares, octad.Black)
		if w != -b {
			t.Errorf("OFEN %s: positionalFeatures white=%.1f black=%.1f (not antisymmetric)", ofen, w, b)
		}
	}
}
