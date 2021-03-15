package engine

import (
	"math"

	"github.com/dechristopher/octad"
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
func Evaluate(situation *octad.Game) float64 {
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

type MoveEval struct {
	Eval float64
	Move octad.Move
}

type SearchAlg int

const MinimaxAB SearchAlg = 0
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
