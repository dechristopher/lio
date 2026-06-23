package engine

import (
	"math"

	"github.com/dechristopher/octad"
	"github.com/pkg/errors"

	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// colorMulti maps a color to the sign used to convert between a
// side-to-move-relative score and an absolute, white-positive score.
var colorMulti = map[octad.Color]int{
	octad.White: 1,
	octad.Black: -1,
}

// searchNegamaxAB is the root for negamax with alpha-beta pruning. It is an
// alternative to the parallel minimax search and selects an equivalent move.
// The returned Eval is absolute and white-positive, matching minimaxABRoot.
func searchNegamaxAB(situation *octad.Game, depth int) MoveEval {
	rootColor := colorMulti[situation.Position().Turn()]
	maxScore := math.Inf(-1)
	var bestMove octad.Move

	for _, move := range situation.ValidMoves() {
		if err := situation.Move(move); err != nil {
			panic(errors.WithMessagef(err,
				"pos: %+v, move: %+v", situation, move))
		}

		// negamaxAB scores the child relative to its side to move (the
		// opponent), so negate to score from the root player's perspective.
		eval := -negamaxAB(situation, move, depth, math.Inf(-1), math.Inf(1))

		situation.UndoMove()

		if eval > maxScore {
			maxScore = eval
			bestMove = *move
		}
	}

	// maxScore is relative to the side to move at the root; convert to
	// absolute white-positive so the result matches the minimax search.
	absEval := float64(rootColor) * maxScore

	util.DebugFlag("engine", str.CEval, "chose best move: %s (%2f) for OFEN: %s",
		bestMove.String(), absEval, situation.Position().String())

	return MoveEval{
		Eval: absEval,
		Move: bestMove,
	}
}

// negamaxAB is the negamax implementation with alpha-beta pruning. It returns
// the value of the node relative to the side to move there, relying on the
// side-to-move-relative convention of Evaluate. Each ply negates the child
// score and swaps/negates the (alpha, beta) window.
func negamaxAB(node *octad.Game, move *octad.Move, depth int, alpha, beta float64) float64 {
	moves := node.ValidMoves()

	if depth == 0 || len(moves) == 0 {
		eval := Evaluate(node)
		util.DebugFlag("eng-v", str.CEval, "negamax: d0|term: move=%s eval=%2f",
			move.String(), eval)
		return eval
	}

	value := math.Inf(-1)
	for _, m := range moves {
		if err := node.Move(m); err != nil {
			panic(errors.WithMessagef(err,
				"pos: %+v, move: %+v", node, m))
		}
		value = math.Max(value, -negamaxAB(node, m, depth-1, -beta, -alpha))
		node.UndoMove()

		alpha = math.Max(alpha, value)

		util.DebugFlag("eng-v", str.CEval, "negamax: d%d: move=%s eval=%2f",
			depth, m.String(), value)

		// fail-high: this node is too good, the opponent will avoid it
		if alpha >= beta {
			break
		}
	}

	return value
}
