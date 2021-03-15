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
		octad.King:   WhiteKingTable,
		octad.Pawn:   WhitePawnTable,
		octad.Knight: WhiteKnightTable,
		octad.Bishop: BishopTable,
		octad.Rook:   RookTable,
		octad.Queen:  QueenTable,
	},
	octad.Black: {
		octad.King:   BlackKingTable,
		octad.Pawn:   BlackPawnTable,
		octad.Knight: BlackKnightTable,
		octad.Bishop: BishopTable,
		octad.Rook:   RookTable,
		octad.Queen:  QueenTable,
	},
}

// WhiteKingTable is the piece square table for the white king
var WhiteKingTable = PieceSquareTable{
	octad.A4: 2, octad.B4: 1, octad.C4: 1, octad.D4: 2,
	octad.A3: 4, octad.B3: 3, octad.C3: 3, octad.D3: 4,
	octad.A2: 5, octad.B2: 4, octad.C2: 4, octad.D2: 5,
	octad.A1: 2, octad.B1: 2, octad.C1: 2, octad.D1: 2,
}

// BlackKingTable is the piece square table for the black king
var BlackKingTable = PieceSquareTable{
	octad.A4: 2, octad.B4: 2, octad.C4: 2, octad.D4: 2,
	octad.A3: 5, octad.B3: 4, octad.C3: 4, octad.D3: 5,
	octad.A2: 4, octad.B2: 3, octad.C2: 3, octad.D2: 4,
	octad.A1: 2, octad.B1: 1, octad.C1: 1, octad.D1: 2,
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

// BishopTable is the piece square table for bishops
var BishopTable = PieceSquareTable{
	octad.A4: 6, octad.B4: 3, octad.C4: 3, octad.D4: 6,
	octad.A3: 3, octad.B3: 10, octad.C3: 10, octad.D3: 3,
	octad.A2: 3, octad.B2: 10, octad.C2: 10, octad.D2: 3,
	octad.A1: 6, octad.B1: 3, octad.C1: 3, octad.D1: 6,
}

// RookTable is the piece square table for rooks
var RookTable = PieceSquareTable{
	octad.A4: 8, octad.B4: 8, octad.C4: 8, octad.D4: 8,
	octad.A3: 8, octad.B3: 8, octad.C3: 8, octad.D3: 8,
	octad.A2: 8, octad.B2: 8, octad.C2: 8, octad.D2: 8,
	octad.A1: 8, octad.B1: 8, octad.C1: 8, octad.D1: 8,
}

// QueenTable is the piece square table for queens
var QueenTable = PieceSquareTable{
	octad.A4: 8, octad.B4: 8, octad.C4: 8, octad.D4: 8,
	octad.A3: 8, octad.B3: 9, octad.C3: 9, octad.D3: 8,
	octad.A2: 8, octad.B2: 9, octad.C2: 9, octad.D2: 8,
	octad.A1: 8, octad.B1: 8, octad.C1: 8, octad.D1: 8,
}
