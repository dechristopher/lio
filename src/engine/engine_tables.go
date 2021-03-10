package engine

import "github.com/dechristopher/octad"

// PieceSquareTable is a map of squares to positional value
// estimates for a given piece type
type PieceSquareTable = map[octad.Square]float64

// PieceTypeTable is a map of piece types to the corresponding
// PieceSquareTable
type PieceTypeTable = map[octad.PieceType]PieceSquareTable

// PieceSquareTables is a collection of PieceSquareTable for
// all piece types for both colors
var PieceSquareTables = map[octad.Color]PieceTypeTable{
	octad.White: {
		octad.Pawn:   WhitePawnTable,
		octad.Knight: WhiteKnightTable,
	},
	octad.Black: {
		octad.Pawn:   BlackPawnTable,
		octad.Knight: BlackKnightTable,
	},
}

// WhitePawnTable is the piece square table for white pawns
var WhitePawnTable = PieceSquareTable{
	octad.A4: 0, octad.B4: 0, octad.C4: 0, octad.D4: 0,
	octad.A3: 10, octad.B3: 10, octad.C3: 10, octad.D3: 10,
	octad.A2: 5, octad.B2: 5, octad.C2: 6, octad.D2: 6,
	octad.A1: 0, octad.B1: 2, octad.C1: 2, octad.D1: 2,
}

// BlackPawnTable is the piece square table for black pawns
var BlackPawnTable = PieceSquareTable{
	octad.A4: 2, octad.B4: 2, octad.C4: 2, octad.D4: 0,
	octad.A3: 6, octad.B3: 6, octad.C3: 5, octad.D3: 5,
	octad.A2: 10, octad.B2: 10, octad.C2: 10, octad.D2: 10,
	octad.A1: 0, octad.B1: 0, octad.C1: 0, octad.D1: 0,
}

// WhiteKnightTable is the piece square table for white knights
var WhiteKnightTable = PieceSquareTable{
	octad.A4: 1, octad.B4: 2, octad.C4: 2, octad.D4: 1,
	octad.A3: 2, octad.B3: 8, octad.C3: 7, octad.D3: 2,
	octad.A2: 2, octad.B2: 7, octad.C2: 10, octad.D2: 2,
	octad.A1: 1, octad.B1: 2, octad.C1: 2, octad.D1: 1,
}

// BlackKnightTable is the piece square table for black knights
var BlackKnightTable = PieceSquareTable{
	octad.A4: 1, octad.B4: 2, octad.C4: 2, octad.D4: 1,
	octad.A3: 2, octad.B3: 10, octad.C3: 7, octad.D3: 2,
	octad.A2: 2, octad.B2: 7, octad.C2: 8, octad.D2: 2,
	octad.A1: 1, octad.B1: 2, octad.C1: 2, octad.D1: 1,
}
