package engine

import (
	"github.com/dechristopher/octad"
)

// MoveEval contains the best move and the evaluation of the best sequence
// of moves to the given depth
type MoveEval struct {
	Eval float64
	Move octad.Move
}

// SearchAlg selector for Search function
type SearchAlg int

// MinimaxAB selects minimax with alpha-beta pruning
const MinimaxAB SearchAlg = 0

// NegamaxAB selects negamax with alpha-beta pruning
const NegamaxAB SearchAlg = 1

// Search returns the best move after running a search algorithm
// on the given position to the given depth
func Search(ofen string, depth int, alg SearchAlg) MoveEval {
	o, err := octad.OFEN(ofen)
	if err != nil {
		panic(err)
	}

	situation, err := octad.NewGame(o)
	if err != nil {
		panic(err)
	}

	if alg == MinimaxAB {
		return searchMinimaxAB(situation, depth)
	} else if alg == NegamaxAB {
		return searchNegamaxAB(situation, depth)
	}

	panic("invalid search algorithm")
}
