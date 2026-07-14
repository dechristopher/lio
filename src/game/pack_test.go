package game

import (
	"testing"

	"github.com/dechristopher/octad/v2"
)

// roundTripMoves round-trips every legal move of a position through
// PackMove/UnpackMoveUOI and reports whether any promotion move was seen.
func roundTripMoves(t *testing.T, ofen string) (promoSeen bool) {
	t.Helper()

	opts := []func(*octad.Game){}
	if ofen != "" {
		opt, err := octad.OFEN(ofen)
		if err != nil {
			t.Fatalf("bad ofen %q: %v", ofen, err)
		}
		opts = append(opts, opt)
	}
	g, err := octad.NewGame(opts...)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}

	for _, m := range g.ValidMoves() {
		if got := UnpackMoveUOI(PackMove(m)); got != m.String() {
			t.Errorf("round-trip mismatch: move=%q packed→%q", m.String(), got)
		}
		if m.Promo() != octad.NoPieceType {
			promoSeen = true
		}
	}
	return promoSeen
}

// TestPackMoveRoundTripStart exercises every from/to square of the opening.
func TestPackMoveRoundTripStart(t *testing.T) {
	roundTripMoves(t, "")
}

// TestPackMoveRoundTripPromotion covers the promotion bits: a white pawn on a3
// promoting to a4 yields all four promotion targets, each of which must survive
// the pack/unpack round trip.
func TestPackMoveRoundTripPromotion(t *testing.T) {
	if !roundTripMoves(t, "3k/P3/4/3K w - - 0 1") {
		t.Fatal("expected at least one promotion move to round-trip")
	}
}
