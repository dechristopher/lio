package engine

import (
	"math"

	"github.com/dechristopher/octad"
	"github.com/pkg/errors"
)

/*
 * TODO
 * - board evaluation function
 *   X piece value
 *   - pawn location
 *      - pawn structure
 *      - rank
 *   - mobility (num legal moves)
 *   - can promote
 *   - can castle
 *   - connectivity (pieces defended)
 *   - king
 *     - king safety (legal moves)
 *
 * X basic minimax search
 *   X alpha-beta pruning
 *   X depth limiting for capping rating
 */

type materialValues = map[octad.Color]float64

// PieceVals contains the material evaluation value
// of each piece type in octad
var PieceVals = map[octad.PieceType]float64{
	octad.King:        100,
	octad.Queen:       9,
	octad.Rook:        5,
	octad.Bishop:      3.1,
	octad.Knight:      3,
	octad.Pawn:        1,
	octad.NoPieceType: 0,
}

// Evaluate returns a numerical evaluation of a game situation
// with positive meaning white winning, negative meaning black
// winning, and zero being a completely drawn game
func Evaluate(situation octad.Game) float64 {
	switch situation.Outcome() {
	case octad.WhiteWon:
		return math.Inf(1)
	case octad.BlackWon:
		return math.Inf(-1)
	case octad.Draw:
		return 0
	default: // continue evaluation if no outcome
		break
	}

	squareMap := situation.Position().Board().SquareMap()

	// calculate material values and piece position values
	material := make(materialValues)
	posVals := make(materialValues)
	for square, piece := range squareMap {
		material[piece.Color()] += PieceVals[piece.Type()]
		// calc piece position vals for pieces with square tables
		if PieceSquareTables[piece.Color()][piece.Type()] != nil {
			posVals[piece.Color()] +=
				PieceSquareTables[piece.Color()][piece.Type()][square]
		}
	}

	// material difference
	md := material[octad.White] - material[octad.Black]

	// positional value difference
	pd := posVals[octad.White] - posVals[octad.Black]

	return md + pd
}

// Search returns the best move after running a search algorithm
// on the given position to the given depth
func Search(situation octad.Game, depth int) MoveEval {
	isWhite := situation.Position().Turn() == octad.White

	bestMoveEval := defBestMoveBlack
	if isWhite {
		bestMoveEval = defBestMoveWhite
	}

	var bestMove MoveEval

	for _, move := range situation.ValidMoves() {
		err := situation.Move(move)
		if err != nil {
			panic(errors.WithMessagef(err,
				"pos: %+v, move: %+v", situation, move))
		}

		eval := minimaxAB(situation, *move, !isWhite,
			depth, math.Inf(-1), math.Inf(1))

		situation.UndoMove()

		if (isWhite && eval.Eval >= bestMoveEval.Eval) ||
			(!isWhite && eval.Eval <= bestMoveEval.Eval) {
			bestMoveEval = eval
			bestMove = MoveEval{
				Eval: eval.Eval,
				Move: *move,
			}
		}
	}

	return bestMove
}

// MoveEval is a struct containing a move and
// its evaluation according to the engine
type MoveEval struct {
	Eval float64
	Move octad.Move
}

var defBestMoveWhite = MoveEval{
	Eval: math.Inf(-1),
}

var defBestMoveBlack = MoveEval{
	Eval: math.Inf(1),
}

// minimaxAB is a recursive minimax search implementation that
// uses alpha-beta pruning to perform search tree cutting and
// subsequently improve the maximum depth we can search in a
// reasonable amount of time
func minimaxAB(
	node octad.Game,
	lastMove octad.Move,
	isMaxi bool,
	depth int,
	alpha, beta float64,
) MoveEval {
	if depth == 0 {
		return MoveEval{
			Eval: Evaluate(node),
			Move: lastMove,
		}
	}

	moves := node.ValidMoves()
	var bestMove MoveEval

	// perform calculations as white (maximizing player)
	if isMaxi {
		bestMove = defBestMoveWhite
		for _, move := range moves {
			err := node.Move(move)
			if err != nil {
				panic(errors.WithMessagef(err,
					"pos: %+v, move: %+v", node, move))
			}

			thisMove := minimaxAB(node, *move, !isMaxi, depth-1, alpha, beta)
			if thisMove.Eval > bestMove.Eval {
				bestMove = thisMove
			}

			node.UndoMove()

			alpha = math.Max(alpha, bestMove.Eval)

			if beta <= alpha {
				break
			}
		}
		return bestMove
	}

	// perform calculations as black (minimizing player)
	bestMove = defBestMoveBlack
	for _, move := range moves {
		err := node.Move(move)
		if err != nil {
			panic(errors.WithMessagef(err,
				"pos: %+v, move: %+v", node, move))
		}

		thisMove := minimaxAB(node, *move, !isMaxi, depth-1, alpha, beta)
		if thisMove.Eval < bestMove.Eval {
			bestMove = thisMove
		}

		node.UndoMove()

		beta = math.Min(beta, bestMove.Eval)

		if beta <= alpha {
			break
		}
	}
	return bestMove
}
