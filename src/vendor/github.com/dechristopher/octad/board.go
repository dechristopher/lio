package octad

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strconv"
	"strings"
)

// A Board represents a octad board and its relationship between squares and pieces.
type Board struct {
	bbWhiteKing   bitboard
	bbWhiteQueen  bitboard
	bbWhiteRook   bitboard
	bbWhiteBishop bitboard
	bbWhiteKnight bitboard
	bbWhitePawn   bitboard
	bbBlackKing   bitboard
	bbBlackQueen  bitboard
	bbBlackRook   bitboard
	bbBlackBishop bitboard
	bbBlackKnight bitboard
	bbBlackPawn   bitboard
	whiteSqs      bitboard
	blackSqs      bitboard
	emptySqs      bitboard
	whiteKingSq   Square
	blackKingSq   Square
}

// NewBoard returns a board from a square to piece mapping.
func NewBoard(m map[Square]Piece) *Board {
	b := &Board{}
	for _, p1 := range allPieces {
		bm := map[Square]bool{}
		for sq, p2 := range m {
			if p1 == p2 {
				bm[sq] = true
			}
		}
		bb := newBitboard(bm)
		b.setBBForPiece(p1, bb)
	}
	b.calcConvenienceBBs(nil)
	return b
}

// SquareMap returns a mapping of squares to pieces. A square is only added to the map if it is occupied.
func (b *Board) SquareMap() map[Square]Piece {
	m := map[Square]Piece{}
	for sq := 0; sq < squaresOnBoard; sq++ {
		p := b.Piece(Square(sq))
		if p != NoPiece {
			m[Square(sq)] = p
		}
	}
	return m
}

// Rotate rotates the board 90 degrees clockwise.
func (b *Board) Rotate() *Board {
	return b.Flip(UpDown).Transpose()
}

// FlipDirection is the direction for the Board.Flip method
type FlipDirection int

const (
	// UpDown flips the board's rank values
	UpDown FlipDirection = iota
	// LeftRight flips the board's file values
	LeftRight
)

// Flip flips the board over the vertical or horizontal
// center line.
func (b *Board) Flip(fd FlipDirection) *Board {
	m := map[Square]Piece{}
	for sq := 0; sq < squaresOnBoard; sq++ {
		var mv Square
		switch fd {
		case UpDown:
			file := Square(sq).File()
			rank := 3 - Square(sq).Rank()
			mv = getSquare(file, rank)
		case LeftRight:
			file := 3 - Square(sq).File()
			rank := Square(sq).Rank()
			mv = getSquare(file, rank)
		}
		m[mv] = b.Piece(Square(sq))
	}
	return NewBoard(m)
}

// Transpose flips the board over the A8 to H1 diagonal.
func (b *Board) Transpose() *Board {
	m := map[Square]Piece{}
	for sq := 0; sq < squaresOnBoard; sq++ {
		file := File(3 - Square(sq).Rank())
		rank := Rank(3 - Square(sq).File())
		mv := getSquare(file, rank)
		m[mv] = b.Piece(Square(sq))
	}
	return NewBoard(m)
}

// Draw returns visual representation of the board useful for debugging.
func (b *Board) Draw() string {
	s := "\n A B C D\n"
	for r := 3; r >= 0; r-- {
		s += Rank(r).String()
		for f := 0; f < squaresInRow; f++ {
			p := b.Piece(getSquare(File(f), Rank(r)))
			if p == NoPiece {
				s += "-"
			} else {
				s += p.String()
			}
			s += " "
		}
		s += "\n"
	}
	return s
}

// String implements the fmt.Stringer interface and returns
// a string in the OFEN board format: ppkn/4/4/NKPP
func (b *Board) String() string {
	fen := ""
	for r := 3; r >= 0; r-- {
		for f := 0; f < squaresInRow; f++ {
			sq := getSquare(File(f), Rank(r))
			p := b.Piece(sq)
			if p != NoPiece {
				fen += p.getOFENChar()
			} else {
				fen += "1"
			}
		}
		if r != 0 {
			fen += "/"
		}
	}
	for i := 4; i > 1; i-- {
		repeatStr := strings.Repeat("1", i)
		countStr := strconv.Itoa(i)
		fen = strings.Replace(fen, repeatStr, countStr, -1)
	}
	return fen
}

// Piece returns the piece for the given square.
func (b *Board) Piece(sq Square) Piece {
	for _, p := range allPieces {
		bb := b.bbForPiece(p)
		if bb.Occupied(sq) {
			return p
		}
	}
	return NoPiece
}

// MarshalText implements the encoding.TextMarshaller interface and returns
// a string in the OFEN board format: ppkn/4/4/NKPP
func (b *Board) MarshalText() (text []byte, err error) {
	return []byte(b.String()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaller interface and takes
// a string in the OFEN board format: ppkn/4/4/NKPP
func (b *Board) UnmarshalText(text []byte) error {
	cp, err := ofenBoard(string(text))
	if err != nil {
		return err
	}
	*b = *cp
	return nil
}

// MarshalBinary implements the encoding.BinaryMarshaller interface and returns
// the bitboard representations as a array of bytes. Bitboards are encoded
// in the following order: WhiteKing, WhiteQueen, WhiteRook, WhiteBishop, WhiteKnight
// WhitePawn, BlackKing, BlackQueen, BlackRook, BlackBishop, BlackKnight, BlackPawn
func (b *Board) MarshalBinary() (data []byte, err error) {
	bbs := []bitboard{b.bbWhiteKing, b.bbWhiteQueen, b.bbWhiteRook, b.bbWhiteBishop, b.bbWhiteKnight, b.bbWhitePawn,
		b.bbBlackKing, b.bbBlackQueen, b.bbBlackRook, b.bbBlackBishop, b.bbBlackKnight, b.bbBlackPawn}
	buf := new(bytes.Buffer)
	err = binary.Write(buf, binary.BigEndian, bbs)
	return buf.Bytes(), err
}

// UnmarshalBinary implements the encoding.BinaryUnmarshaler interface and parses
// the bitboard representations as a array of bytes. Bitboards are decoded
// in the following order: WhiteKing, WhiteQueen, WhiteRook, WhiteBishop, WhiteKnight
// WhitePawn, BlackKing, BlackQueen, BlackRook, BlackBishop, BlackKnight, BlackPawn
func (b *Board) UnmarshalBinary(data []byte) error {
	if len(data) != 24 {
		return errors.New("octad: invalid number of bytes for board unmarshal binary")
	}
	b.bbWhiteKing = bitboard(binary.BigEndian.Uint16(data[:2]))
	b.bbWhiteQueen = bitboard(binary.BigEndian.Uint16(data[2:4]))
	b.bbWhiteRook = bitboard(binary.BigEndian.Uint16(data[4:6]))
	b.bbWhiteBishop = bitboard(binary.BigEndian.Uint16(data[6:8]))
	b.bbWhiteKnight = bitboard(binary.BigEndian.Uint16(data[8:10]))
	b.bbWhitePawn = bitboard(binary.BigEndian.Uint16(data[10:12]))
	b.bbBlackKing = bitboard(binary.BigEndian.Uint16(data[12:14]))
	b.bbBlackQueen = bitboard(binary.BigEndian.Uint16(data[14:16]))
	b.bbBlackRook = bitboard(binary.BigEndian.Uint16(data[16:18]))
	b.bbBlackBishop = bitboard(binary.BigEndian.Uint16(data[18:20]))
	b.bbBlackKnight = bitboard(binary.BigEndian.Uint16(data[20:22]))
	b.bbBlackPawn = bitboard(binary.BigEndian.Uint16(data[22:24]))
	b.calcConvenienceBBs(nil)
	return nil
}

func (b *Board) update(m *Move) {
	p1 := b.Piece(m.s1)
	s1BB := bbForSquare(m.s1)
	s2BB := bbForSquare(m.s2)

	// move s1 piece to s2
	for _, p := range allPieces {
		bb := b.bbForPiece(p)
		// remove what was at s2
		b.setBBForPiece(p, bb & ^s2BB)
		// move what was at s1 to s2
		if bb.Occupied(m.s1) {
			bb = b.bbForPiece(p)
			b.setBBForPiece(p, (bb & ^s1BB)|s2BB)
		}
	}
	// check promotion
	if m.promo != NoPieceType {
		newPiece := getPiece(m.promo, p1.Color())
		// remove pawn
		bbPawn := b.bbForPiece(p1)
		b.setBBForPiece(p1, bbPawn & ^s2BB)
		// add promo piece
		bbPromo := b.bbForPiece(newPiece)
		b.setBBForPiece(newPiece, bbPromo|s2BB)
	}
	// remove captured en passant piece
	if m.HasTag(EnPassant) {
		if p1.Color() == White {
			b.bbBlackPawn = ^(bbForSquare(m.s2) << 4) & b.bbBlackPawn
		} else {
			b.bbWhitePawn = ^(bbForSquare(m.s2) >> 4) & b.bbWhitePawn
		}
	}
	// move pieces while castling
	moveCastledPieces(b, p1, m)
	b.calcConvenienceBBs(m)
}

func moveCastledPieces(b *Board, p1 Piece, m *Move) {
	if p1.Color() == White && m.HasTag(KnightCastle) {
		b.bbWhiteKnight = (b.bbWhiteKnight & ^bbForSquare(A1)) | bbForSquare(B1)
	} else if p1.Color() == White && m.HasTag(ClosePawnCastle) {
		b.bbWhitePawn = (b.bbWhitePawn & ^bbForSquare(C1)) | bbForSquare(B1)
	} else if p1.Color() == White && m.HasTag(FarPawnCastle) {
		b.bbWhitePawn = (b.bbWhitePawn & ^bbForSquare(D1)) | bbForSquare(B1)
		// finagle the king back to C1 since move is technically to D1
		b.bbWhiteKing = (b.bbWhiteKing & ^bbForSquare(D1)) | bbForSquare(C1)
	} else if p1.Color() == Black && m.HasTag(KnightCastle) {
		b.bbBlackKnight = (b.bbBlackKnight & ^bbForSquare(D4)) | bbForSquare(C4)
	} else if p1.Color() == Black && m.HasTag(ClosePawnCastle) {
		b.bbBlackPawn = (b.bbBlackPawn & ^bbForSquare(B4)) | bbForSquare(C4)
	} else if p1.Color() == Black && m.HasTag(FarPawnCastle) {
		b.bbBlackPawn = (b.bbBlackPawn & ^bbForSquare(A4)) | bbForSquare(C4)
		// finagle the king back to B4 since move is technically to A4
		b.bbBlackKing = (b.bbBlackKing ^ bbForSquare(A4)) | bbForSquare(B4)
	}
}

func (b *Board) calcConvenienceBBs(m *Move) {
	whiteSqs := b.bbWhiteKing | b.bbWhiteQueen | b.bbWhiteRook | b.bbWhiteBishop | b.bbWhiteKnight | b.bbWhitePawn
	blackSqs := b.bbBlackKing | b.bbBlackQueen | b.bbBlackRook | b.bbBlackBishop | b.bbBlackKnight | b.bbBlackPawn
	emptySqs := ^(whiteSqs | blackSqs)
	b.whiteSqs = whiteSqs
	b.blackSqs = blackSqs
	b.emptySqs = emptySqs
	if m == nil {
		b.whiteKingSq = NoSquare
		b.blackKingSq = NoSquare

		for sq := 0; sq < squaresOnBoard; sq++ {
			sqr := Square(sq)
			if b.bbWhiteKing.Occupied(sqr) {
				b.whiteKingSq = sqr
			} else if b.bbBlackKing.Occupied(sqr) {
				b.blackKingSq = sqr
			}
		}
	} else if m.s1 == b.whiteKingSq {
		b.whiteKingSq = m.s2
		if m.HasTag(FarPawnCastle) {
			b.whiteKingSq = C1
		}
	} else if m.s1 == b.blackKingSq {
		b.blackKingSq = m.s2
		if m.HasTag(FarPawnCastle) {
			b.blackKingSq = B4
		}
	}
}

func (b *Board) copy() *Board {
	return &Board{
		whiteSqs:      b.whiteSqs,
		blackSqs:      b.blackSqs,
		emptySqs:      b.emptySqs,
		whiteKingSq:   b.whiteKingSq,
		blackKingSq:   b.blackKingSq,
		bbWhiteKing:   b.bbWhiteKing,
		bbWhiteQueen:  b.bbWhiteQueen,
		bbWhiteRook:   b.bbWhiteRook,
		bbWhiteBishop: b.bbWhiteBishop,
		bbWhiteKnight: b.bbWhiteKnight,
		bbWhitePawn:   b.bbWhitePawn,
		bbBlackKing:   b.bbBlackKing,
		bbBlackQueen:  b.bbBlackQueen,
		bbBlackRook:   b.bbBlackRook,
		bbBlackBishop: b.bbBlackBishop,
		bbBlackKnight: b.bbBlackKnight,
		bbBlackPawn:   b.bbBlackPawn,
	}
}

func (b *Board) isOccupied(sq Square) bool {
	return !b.emptySqs.Occupied(sq)
}

func (b *Board) hasSufficientMaterial() bool {
	// queen, rook, or pawn exist
	if (b.bbWhiteQueen | b.bbWhiteRook | b.bbWhitePawn |
		b.bbBlackQueen | b.bbBlackRook | b.bbBlackPawn) > 0 {
		return true
	}
	// if king is missing then it is a test
	if b.bbWhiteKing == 0 || b.bbBlackKing == 0 {
		return true
	}
	count := map[PieceType]int{}
	pieceMap := b.SquareMap()
	for _, p := range pieceMap {
		count[p.Type()]++
	}
	// 	king versus king
	if count[Bishop] == 0 && count[Knight] == 0 {
		return false
	}
	// king and bishop versus king
	if count[Bishop] == 1 && count[Knight] == 0 {
		return false
	}
	// king and knight versus king
	if count[Bishop] == 0 && count[Knight] == 1 {
		return false
	}
	// king and bishop(s) versus king and bishop(s) with the bishops on the same colour.
	if count[Knight] == 0 {
		whiteCount := 0
		blackCount := 0
		for sq, p := range pieceMap {
			if p.Type() == Bishop {
				switch sq.Color() {
				case White:
					whiteCount++
				case Black:
					blackCount++
				}
			}
		}
		if whiteCount == 0 || blackCount == 0 {
			return false
		}
	}
	return true
}

func (b *Board) bbForPiece(p Piece) bitboard {
	switch p {
	case WhiteKing:
		return b.bbWhiteKing
	case WhiteQueen:
		return b.bbWhiteQueen
	case WhiteRook:
		return b.bbWhiteRook
	case WhiteBishop:
		return b.bbWhiteBishop
	case WhiteKnight:
		return b.bbWhiteKnight
	case WhitePawn:
		return b.bbWhitePawn
	case BlackKing:
		return b.bbBlackKing
	case BlackQueen:
		return b.bbBlackQueen
	case BlackRook:
		return b.bbBlackRook
	case BlackBishop:
		return b.bbBlackBishop
	case BlackKnight:
		return b.bbBlackKnight
	case BlackPawn:
		return b.bbBlackPawn
	}
	return bitboard(0)
}

func (b *Board) setBBForPiece(p Piece, bb bitboard) {
	switch p {
	case WhiteKing:
		b.bbWhiteKing = bb
	case WhiteQueen:
		b.bbWhiteQueen = bb
	case WhiteRook:
		b.bbWhiteRook = bb
	case WhiteBishop:
		b.bbWhiteBishop = bb
	case WhiteKnight:
		b.bbWhiteKnight = bb
	case WhitePawn:
		b.bbWhitePawn = bb
	case BlackKing:
		b.bbBlackKing = bb
	case BlackQueen:
		b.bbBlackQueen = bb
	case BlackRook:
		b.bbBlackRook = bb
	case BlackBishop:
		b.bbBlackBishop = bb
	case BlackKnight:
		b.bbBlackKnight = bb
	case BlackPawn:
		b.bbBlackPawn = bb
	default:
		panic("invalid piece")
	}
}
