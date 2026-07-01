package game

import (
	"testing"

	"github.com/dechristopher/lio/variant"
)

// TestHistoryHelpers verifies the SAN/OFEN per-ply history the server sends to
// clients for the move list + review navigation: OFENHistory has one more entry
// than the move count (index 0 is the start position), SANHistory is parallel to
// MoveHistory, and the final OFEN matches the live position.
func TestHistoryHelpers(t *testing.T) {
	const startOFEN = "ppkn/4/4/NKPP w NCFncf - 0 1"

	g, err := NewOctadGame(OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		White:   "w",
		Black:   "b",
	})
	if err != nil {
		t.Fatalf("NewOctadGame failed: %v", err)
	}

	// before any move: no SANs, a single OFEN (the start position)
	if len(g.SANHistory()) != 0 {
		t.Fatalf("expected empty SAN history at start, got %v", g.SANHistory())
	}
	if ofens := g.OFENHistory(); len(ofens) != 1 || ofens[0] != startOFEN {
		t.Fatalf("start OFEN history = %v, want [%q]", ofens, startOFEN)
	}

	// play a handful of real moves
	plies := 0
	for i := 0; i < 6; i++ {
		moves := g.Game.ValidMoves()
		if len(moves) == 0 {
			break
		}
		if err := g.Game.Move(moves[0]); err != nil {
			t.Fatalf("playing move %s failed: %v", moves[0], err)
		}
		plies++
	}
	if plies == 0 {
		t.Fatal("could not play any moves")
	}

	uois := g.MoveHistory()
	sans := g.SANHistory()
	ofens := g.OFENHistory()

	if len(sans) != len(uois) {
		t.Fatalf("SAN history length %d != move history length %d", len(sans), len(uois))
	}
	if len(ofens) != len(uois)+1 {
		t.Fatalf("OFEN history length %d, want plies+1 = %d", len(ofens), len(uois)+1)
	}
	if ofens[0] != startOFEN {
		t.Fatalf("OFEN history[0] = %q, want start %q", ofens[0], startOFEN)
	}
	// the last OFEN entry must equal the live position
	if ofens[len(ofens)-1] != g.OFEN() {
		t.Fatalf("final OFEN history %q != live OFEN %q", ofens[len(ofens)-1], g.OFEN())
	}
	// every SAN must be non-empty
	for i, s := range sans {
		if s == "" {
			t.Fatalf("SAN history[%d] is empty (uoi %q)", i, uois[i])
		}
	}
}
