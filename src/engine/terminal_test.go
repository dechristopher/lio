package engine

import (
	"testing"
	"time"
)

// Terminal positions (no legal moves) reach Search via the background position
// evaluator, which feeds it every archived game's *final* position. These are
// real final OFENs produced by random self-play; both have zero legal moves
// for the side to move.
const (
	// white to move and checkmated (black won)
	mateOFEN = "4/2k1/2P1/1q1K w - - 4 16"
	// black to move and stalemated (draw)
	stalemateOFEN = "4/2KN/4/N2k b - - 8 12"
)

// TestSearchTerminalPosition is the regression test for the evaluator panic
// ("index out of range [0] with length 0" in bestOf): Search on a position
// with no legal moves must return the zero move and the exact terminal eval
// in absolute white-positive space — never panic. The budgeted call is the
// deepeningRoot path the crash came through; the unbudgeted call covers the
// fixed-depth root, and Random covers its rng.Intn(0) hazard.
func TestSearchTerminalPosition(t *testing.T) {
	cases := []struct {
		name   string
		ofen   string
		budget time.Duration
		alg    SearchAlg
		want   float64
	}{
		{"mate budgeted (evaluator path)", mateOFEN, 100 * time.Millisecond, MinimaxAB, -WinVal},
		{"mate unbudgeted", mateOFEN, 0, MinimaxAB, -WinVal},
		{"mate random alg", mateOFEN, 0, Random, -WinVal},
		{"stalemate budgeted", stalemateOFEN, 100 * time.Millisecond, MinimaxAB, 0},
		{"stalemate unbudgeted", stalemateOFEN, 0, MinimaxAB, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			me := Search(tc.ofen, nil, 4, tc.budget, tc.alg)
			if got := me.Move.String(); got != "a1a1" {
				t.Fatalf("terminal position returned a move %q, want the zero move", got)
			}
			if me.Eval != tc.want {
				t.Fatalf("terminal eval = %v, want %v (white-positive)", me.Eval, tc.want)
			}
		})
	}
}

// TestBestOfEmptyMoves guards the defensive layer beneath Search: an empty
// move list yields the zero MoveEval instead of indexing moves[0].
func TestBestOfEmptyMoves(t *testing.T) {
	for _, isWhite := range []bool{true, false} {
		me := bestOf(nil, nil, isWhite)
		if me.Move.String() != "a1a1" || me.Eval != 0 {
			t.Fatalf("bestOf(empty) = %+v, want zero MoveEval", me)
		}
	}
}
