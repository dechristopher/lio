package engine

import "github.com/dechristopher/octad"

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
 * - basic minimax search
 *   - alpha-beta pruning
 *
 * - depth limiting for capping rating
 */

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
	squareMap := situation.Position().Board().SquareMap()

	// calculate material values
	material := make(map[octad.Color]float64)
	for _, piece := range squareMap {
		material[piece.Color()] += PieceVals[piece.Type()]
	}

	// material difference
	md := material[octad.White] - material[octad.Black]

	return md
}
