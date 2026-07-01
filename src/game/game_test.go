package game

import (
	"testing"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/variant"
)

// TestPlayFromDeployedOFEN is the Phase 0 spike for the blind Deploy Phase. It
// proves the central premise: the octad library + lio game model construct and
// play a game correctly starting from a non-standard, permuted home-rank
// position (the eventual output of a deploy phase), with no library changes.
//
// White is deployed as PNKP (a1=P,b1=N,c1=K,d1=P), Black as knpp
// (a4=k,b4=n,c4=p,d4=p). Castle rights are dropped ("-") here because
// position-relative castling is Phase 1; this spike only validates that an
// arbitrary legal placement plays.
func TestPlayFromDeployedOFEN(t *testing.T) {
	const deployedOFEN = "knpp/4/4/PNKP w - - 0 1"

	g, err := NewOctadGame(OctadGameConfig{
		Variant: variant.HalfOneBlitz,
		OFEN:    deployedOFEN,
		White:   "w",
		Black:   "b",
	})
	if err != nil {
		t.Fatalf("NewOctadGame from deployed OFEN failed: %v", err)
	}

	// OFEN must round-trip exactly through parse + serialize.
	if got := g.OFEN(); got != deployedOFEN {
		t.Fatalf("OFEN round-trip mismatch: got %q want %q", got, deployedOFEN)
	}

	if g.ToMove != octad.White {
		t.Fatalf("expected White to move, got %v", g.ToMove)
	}

	// Legal moves must generate from the deployed position, including the
	// double pawn push a1->a3 (octad pawns double-move off their home rank).
	legal := g.LegalMoves()
	if len(legal) == 0 {
		t.Fatal("no legal moves from deployed position")
	}
	if !contains(legal["a1"], "a3") {
		t.Fatalf("expected a1->a3 double push available, got a1 dests %v", legal["a1"])
	}

	// Play several real moves to confirm the position advances normally.
	plies := 0
	for i := 0; i < 6; i++ {
		moves := g.Game.ValidMoves()
		if len(moves) == 0 {
			break // terminal position reached; fine for the spike
		}
		if err := g.Game.Move(moves[0]); err != nil {
			t.Fatalf("playing move %s failed: %v", moves[0], err)
		}
		plies++
	}
	if plies == 0 {
		t.Fatal("could not play any move from deployed position")
	}

	if len(g.MoveHistory()) != plies {
		t.Fatalf("move history length %d, expected %d", len(g.MoveHistory()), plies)
	}
	if g.OFEN() == deployedOFEN {
		t.Fatal("OFEN did not change after playing moves")
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
