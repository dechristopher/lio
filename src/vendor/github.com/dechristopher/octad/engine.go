package octad

type engine struct{}

func (engine) CalcMoves(pos *Position, first bool) []*Move {
	// generate possible moves
	moves := standardMoves(pos, first)
	// return moves including castles
	return append(moves, castleMoves(pos)...)
}

func (engine) Status(pos *Position) Method {
	var hasMove bool
	if pos.validMoves != nil {
		hasMove = len(pos.validMoves) > 0
	} else {
		hasMove = len(engine{}.CalcMoves(pos, true)) > 0
	}
	if !pos.inCheck && !hasMove {
		return Stalemate
	} else if pos.inCheck && !hasMove {
		return Checkmate
	}
	return NoMethod
}

var (
	promoPieceTypes = []PieceType{Queen, Rook, Bishop, Knight}
)

func standardMoves(pos *Position, first bool) []*Move {
	// compute allowed destination bitboard
	bbAllowed := ^pos.board.whiteSqs
	if pos.Turn() == Black {
		bbAllowed = ^pos.board.blackSqs
	}
	var moves []*Move
	// iterate through pieces to find possible moves
	for _, p := range allPieces {
		if pos.Turn() != p.Color() {
			continue
		}
		// iterate through possible starting squares for piece
		s1BB := pos.board.bbForPiece(p)
		if s1BB == 0 {
			continue
		}
		for s1 := 0; s1 < squaresOnBoard; s1++ {
			if s1BB&bbForSquare(Square(s1)) == 0 {
				continue
			}
			// iterate through possible destination squares for piece
			s2BB := bbForPossibleMoves(pos, p.Type(), Square(s1)) & bbAllowed
			if s2BB == 0 {
				continue
			}
			for s2 := 0; s2 < squaresOnBoard; s2++ {
				if s2BB&bbForSquare(Square(s2)) == 0 {
					continue
				}
				// add promotions if pawn on promo square
				if (p == WhitePawn && Square(s2).Rank() == Rank4) || (p == BlackPawn && Square(s2).Rank() == Rank1) {
					for _, pt := range promoPieceTypes {
						m := &Move{s1: Square(s1), s2: Square(s2), promo: pt}
						addTags(m, pos)
						// filter out moves that put king into check
						if !m.HasTag(inCheck) {
							moves = append(moves, m)
							if first {
								return moves
							}
						}
					}
				} else {
					m := &Move{s1: Square(s1), s2: Square(s2)}
					addTags(m, pos)
					// filter out moves that put king into check
					if !m.HasTag(inCheck) {
						moves = append(moves, m)
						if first {
							return moves
						}
					}
				}
			}
		}
	}
	return moves
}

func addTags(m *Move, pos *Position) {
	p := pos.board.Piece(m.s1)
	if pos.board.isOccupied(m.s2) {
		m.addTag(Capture)
	} else if m.s2 == pos.enPassantSquare && p.Type() == Pawn {
		m.addTag(EnPassant)
	}
	// determine if in check after move (makes move invalid)
	cp := pos.copy()
	cp.board.update(m)
	if isInCheck(cp) {
		m.addTag(inCheck)
	}
	// determine if opponent in check after move
	cp.turn = cp.turn.Other()
	if isInCheck(cp) {
		m.addTag(Check)
	}
}

func isInCheck(pos *Position) bool {
	kingSq := pos.activeKingSquare()
	// king should only be missing in tests / examples
	if kingSq == NoSquare {
		return false
	}
	return squaresAreAttacked(pos, kingSq)
}

func squaresAreAttacked(pos *Position, sqs ...Square) bool {
	otherColor := pos.Turn().Other()
	occ := ^pos.board.emptySqs
	for _, sq := range sqs {
		// hot path check to see if attack vector is possible
		s2BB := pos.board.blackSqs
		if pos.Turn() == Black {
			s2BB = pos.board.whiteSqs
		}
		if ((diaAttack(occ, sq)|hvAttack(occ, sq))&s2BB)|(bbKnightMoves[sq]&s2BB) == 0 {
			continue
		}
		// check queen attack vector
		queenBB := pos.board.bbForPiece(getPiece(Queen, otherColor))
		bb := (diaAttack(occ, sq) | hvAttack(occ, sq)) & queenBB
		if bb != 0 {
			return true
		}
		// check rook attack vector
		rookBB := pos.board.bbForPiece(getPiece(Rook, otherColor))
		bb = hvAttack(occ, sq) & rookBB
		if bb != 0 {
			return true
		}
		// check bishop attack vector
		bishopBB := pos.board.bbForPiece(getPiece(Bishop, otherColor))
		bb = diaAttack(occ, sq) & bishopBB
		if bb != 0 {
			return true
		}
		// check knight attack vector
		knightBB := pos.board.bbForPiece(getPiece(Knight, otherColor))
		bb = bbKnightMoves[sq] & knightBB
		if bb != 0 {
			return true
		}
		// check pawn attack vector
		if pos.Turn() == White {
			capRight := (pos.board.bbBlackPawn & ^bbFileD & ^bbRank1) << 3
			capLeft := (pos.board.bbBlackPawn & ^bbFileA & ^bbRank1) << 5
			bb = (capRight | capLeft) & bbForSquare(sq)
			if bb != 0 {
				return true
			}
		} else {
			capRight := (pos.board.bbWhitePawn & ^bbFileD & ^bbRank4) >> 5
			capLeft := (pos.board.bbWhitePawn & ^bbFileA & ^bbRank4) >> 3
			bb = (capRight | capLeft) & bbForSquare(sq)
			if bb != 0 {
				return true
			}
		}
		// check king attack vector
		kingBB := pos.board.bbForPiece(getPiece(King, otherColor))
		bb = bbKingMoves[sq] & kingBB
		if bb != 0 {
			return true
		}
	}
	return false
}

func bbForPossibleMoves(pos *Position, pt PieceType, sq Square) bitboard {
	switch pt {
	case King:
		return bbKingMoves[sq]
	case Queen:
		return diaAttack(^pos.board.emptySqs, sq) | hvAttack(^pos.board.emptySqs, sq)
	case Rook:
		return hvAttack(^pos.board.emptySqs, sq)
	case Bishop:
		return diaAttack(^pos.board.emptySqs, sq)
	case Knight:
		return bbKnightMoves[sq]
	case Pawn:
		return pawnMoves(pos, sq)
	}
	return bitboard(0)
}

// TODO can calc isInCheck twice
func castleMoves(pos *Position) []*Move {
	var moves []*Move

	knightSide := pos.castleRights.CanCastle(pos.Turn(), KnightSide)
	closeSide := pos.castleRights.CanCastle(pos.Turn(), CloseSide)
	farSide := pos.castleRights.CanCastle(pos.Turn(), FarSide)

	if pos.turn == White {
		// white knight side
		if knightSide &&
			!squaresAreAttacked(pos, A1) &&
			!pos.inCheck {
			m := &Move{s1: B1, s2: A1}
			m.addTag(KnightCastle)
			addTags(m, pos)
			moves = append(moves, m)
		}
		// white far side
		if farSide &&
			(^pos.board.emptySqs&(bbForSquare(C1))) == 0 &&
			!squaresAreAttacked(pos, C1) &&
			!pos.inCheck {
			m := &Move{s1: B1, s2: D1}
			m.addTag(FarPawnCastle)
			addTags(m, pos)
			moves = append(moves, m)
		}
		// white close side
		if closeSide &&
			!squaresAreAttacked(pos, C1) &&
			!pos.inCheck {
			m := &Move{s1: B1, s2: C1}
			m.addTag(ClosePawnCastle)
			addTags(m, pos)
			moves = append(moves, m)
		}
	} else {
		// black knight side
		if knightSide &&
			!squaresAreAttacked(pos, D4) &&
			!pos.inCheck {
			m := &Move{s1: C4, s2: D4}
			m.addTag(KnightCastle)
			addTags(m, pos)
			moves = append(moves, m)
		}
		// black far side
		if farSide &&
			(^pos.board.emptySqs&(bbForSquare(B4))) == 0 &&
			!squaresAreAttacked(pos, B4) &&
			!pos.inCheck {
			m := &Move{s1: C4, s2: A4}
			m.addTag(FarPawnCastle)
			addTags(m, pos)
			moves = append(moves, m)
		}
		// black close side
		if closeSide &&
			!squaresAreAttacked(pos, B4) &&
			!pos.inCheck {
			m := &Move{s1: C4, s2: B4}
			m.addTag(ClosePawnCastle)
			addTags(m, pos)
			moves = append(moves, m)
		}
	}
	return moves
}

func pawnMoves(pos *Position, sq Square) bitboard {
	bb := bbForSquare(sq)
	var bbEnPassant bitboard
	if pos.enPassantSquare != NoSquare {
		bbEnPassant = bbForSquare(pos.enPassantSquare)
	}

	if pos.Turn() == White {
		capRight := ((bb & ^bbFileD & ^bbRank4) >> 5) & (pos.board.blackSqs | bbEnPassant)
		capLeft := ((bb & ^bbFileA & ^bbRank4) >> 3) & (pos.board.blackSqs | bbEnPassant)
		upOne := ((bb & ^bbRank4) >> 4) & pos.board.emptySqs
		upTwo := ((upOne & bbRank2) >> 4) & pos.board.emptySqs
		return capRight | capLeft | upOne | upTwo
	}

	capRight := ((bb & ^bbFileD & ^bbRank1) << 3) & (pos.board.whiteSqs | bbEnPassant)
	capLeft := ((bb & ^bbFileA & ^bbRank1) << 5) & (pos.board.whiteSqs | bbEnPassant)
	upOne := ((bb & ^bbRank1) << 4) & pos.board.emptySqs
	upTwo := ((upOne & bbRank3) << 4) & pos.board.emptySqs
	return capRight | capLeft | upOne | upTwo
}

func diaAttack(occupied bitboard, sq Square) bitboard {
	pos := bbForSquare(sq)
	dMask := bbDiagonals[sq]
	adMask := bbAntiDiagonals[sq]
	return linearAttack(occupied, pos, dMask) | linearAttack(occupied, pos, adMask)
}

func hvAttack(occupied bitboard, sq Square) bitboard {
	pos := bbForSquare(sq)
	rankMask := bbRanks[sq.Rank()]
	fileMask := bbFiles[sq.File()]
	return linearAttack(occupied, pos, rankMask) | linearAttack(occupied, pos, fileMask)
}

func linearAttack(occupied, pos, mask bitboard) bitboard {
	oInMask := occupied & mask
	return ((oInMask - 2*pos) ^ (oInMask.Reverse() - 2*pos.Reverse()).Reverse()) & mask
}

const (
	bbFileA bitboard = 34952
	bbFileB bitboard = 17476
	bbFileC bitboard = 8738
	bbFileD bitboard = 4369

	bbRank1 bitboard = 61440
	bbRank2 bitboard = 3840
	bbRank3 bitboard = 240
	bbRank4 bitboard = 15
)

// TODO make method on Square
func bbForSquare(sq Square) bitboard {
	return bbSquares[sq]
}

var (
	bbFiles = [4]bitboard{bbFileA, bbFileB, bbFileC, bbFileD}
	bbRanks = [4]bitboard{bbRank1, bbRank2, bbRank3, bbRank4}

	// bbDiagonals represents bottom-left to top-right diagonal
	// bitboards calculated per square
	bbDiagonals = [16]bitboard{
		33825, // A1
		16912, // B1
		8448,  // C1
		4096,  // D1
		2114,  // A2
		33825, // B2
		16912, // C2
		8448,  // D2
		132,   // A3
		2114,  // B3
		33825, // C3
		16912, // D3
		8,     // A4
		132,   // B4
		2114,  // C4
		33825, // D4
	}

	// bbAntiDiagonals represents bottom-right to top-left diagonal
	// bitboards calculated per square
	bbAntiDiagonals = [16]bitboard{
		32768, // A1
		18432, // B1
		9344,  // C1
		4680,  // D1
		18432, // A2
		9344,  // B2
		4680,  // C2
		292,   // D2
		9344,  // A3
		4680,  // B3
		292,   // C3
		18,    // D3
		4680,  // A4
		292,   // B4
		18,    // C4
		1,     // D4
	}

	bbKnightMoves = [16]bitboard{
		576,   // A1
		416,   // B1
		2128,  // C1
		1056,  // D1
		8228,  // A2
		4122,  // B2
		32901, // C2
		16450, // D2
		16898, // A3
		41217, // B3
		22536, // C3
		9220,  // D3
		1056,  // A4
		2576,  // B4
		1408,  // C4
		576,   // D4
	}

	bbKingMoves = [16]bitboard{
		19456, // A1
		44544, // B1
		22272, // C1
		8960,  // D1
		50368, // A2
		60128, // B2
		30064, // C2
		12848, // D2
		3148,  // A3
		3758,  // B3
		1879,  // C3
		803,   // D3
		196,   // A4
		234,   // B4
		117,   // C4
		50,    // D4
	}

	bbSquares = [16]bitboard{}
)

func init() {
	for sq := 0; sq < 16; sq++ {
		bbSquares[sq] = bitboard(uint16(1) << (uint8(15) - uint8(sq)))
	}
}
