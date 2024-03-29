package engine

import (
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

const WinVal = 10000

// PieceVals contains the material evaluation value
// of each piece type in octad
var PieceVals = map[octad.PieceType]float64{
	octad.King:        1000,
	octad.Queen:       90,
	octad.Rook:        50,
	octad.Bishop:      31,
	octad.Knight:      30,
	octad.Pawn:        10,
	octad.NoPieceType: 0,
}

// Evaluate returns a numerical evaluation of a game situation
// with positive meaning white winning, negative meaning black
// winning, and zero being a completely drawn game
func Evaluate(situation *octad.Game) float64 {
	color := situation.Position().Turn()

	switch situation.Outcome() {
	case octad.WhiteWon:
		if color == octad.White {
			return WinVal
		}
		return -WinVal
	case octad.BlackWon:
		if color == octad.White {
			return -WinVal
		}
		return WinVal
	case octad.Draw:
		return 0.0
	default: // continue evaluation if no outcome
		break
	}

	eval := 0.0

	if situation.Position().InCheck() {
		if color == octad.White {
			eval -= 1000
		} else {
			eval += 1000
		}
	}

	squareMap := situation.Position().Board().SquareMap()

	// calculate material values and piece position values
	material := make(materialValues)
	posValues := make(materialValues)
	for square, piece := range squareMap {
		material[piece.Color()] += PieceVals[piece.Type()]
		// calc piece position values for pieces with square tables
		if PieceSquareTables[piece.Color()][piece.Type()] != nil {
			posValues[piece.Color()] +=
				PieceSquareTables[piece.Color()][piece.Type()][square]
		}
	}

	// material difference
	eval += material[color] - material[color.Other()]

	// positional value difference
	eval += posValues[color] - posValues[color.Other()]

	return eval
}
