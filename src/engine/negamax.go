package engine

import (
	"math"

	"github.com/dechristopher/octad"
	"github.com/pkg/errors"

	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// searchNegamaxAB is the root for negamax with alpha-beta pruning
func searchNegamaxAB(situation *octad.Game, depth int) MoveEval {
	color := colorMulti[situation.Position().Turn()]
	maxScore := math.Inf(-1)
	var bestMove octad.Move

	moves := situation.ValidMoves()

	for _, move := range moves {
		err := situation.Move(move)
		if err != nil {
			panic(errors.WithMessagef(err,
				"pos: %+v, move: %+v", situation, move))
		}

		eval := negamaxAB(situation, move, depth,
			math.Inf(1), math.Inf(-1), color)

		situation.UndoMove()

		if eval >= maxScore {
			maxScore = eval
			bestMove = *move
		}
	}

	util.Debug(str.CEval, "chose best move: %s (%2f) for OFEN: %s",
		bestMove.String(), maxScore, situation.Position().String())

	return MoveEval{
		Eval: maxScore,
		Move: bestMove,
	}
}

var colorMulti = map[octad.Color]int{
	octad.White: 1,
	octad.Black: -1,
}

// negamax algorithm implementation with no pruning or deepening
func negamaxAB(node *octad.Game, move *octad.Move, depth int, alpha, beta float64, toMove int) float64 {
	moves := node.ValidMoves()

	if depth == 0 || len(moves) == 0 {
		eval := float64(toMove) * Evaluate(node)
		util.Debug(str.CEval, "negamax: d0|term: move=%s eval=%2f",
			move.String(), eval)
		return eval
	}

	eval := math.Inf(-1)
	for _, m := range moves {
		err := node.Move(m)
		if err != nil {
			panic(errors.WithMessagef(err,
				"pos: %+v, move: %+v", node, move))
		}
		score := math.Max(eval, -negamaxAB(node, m, depth-1, -beta, -alpha, -toMove))
		alpha = math.Max(alpha, score)

		node.UndoMove()

		util.Debug(str.CEval, "negamax: d%d: move=%s eval=%2f",
			depth, move.String(), score)

		if alpha >= beta {
			break
		}
	}
	return eval
}
