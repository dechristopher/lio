package octad

import (
	"bytes"
	"crypto/md5"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Side represents a side to castle to. In octad, there are three types of
// castling allowed. Knight, close pawn, and far pawn
type Side int

const (
	// KnightSide is castling with the knight
	KnightSide Side = iota + 1
	// CloseSide is castling with the close pawn
	CloseSide
	// FarSide is castling with the far pawn
	FarSide
)

// CastleRights holds the state of both sides castling abilities.
type CastleRights string

// CanCastle returns true if the given color and side combination
// can castle, otherwise returns false.
func (cr CastleRights) CanCastle(c Color, side Side) bool {
	char := "n"
	if side == CloseSide {
		char = "c"
	}
	if side == FarSide {
		char = "f"
	}
	if c == White {
		char = strings.ToUpper(char)
	}
	return strings.Contains(string(cr), char)
}

// String implements the fmt.Stringer interface and returns
// a FEN compatible string. Ex. NCFncf
func (cr CastleRights) String() string {
	return string(cr)
}

// Position represents the state of the game without regard
// to its outcome. Position is translatable to FEN notation.
type Position struct {
	board           *Board
	turn            Color
	castleRights    CastleRights
	enPassantSquare Square
	halfMoveClock   int
	moveCount       int
	inCheck         bool
	validMoves      []*Move
}

const (
	startOFEN = "ppkn/4/4/NKPP w NCFncf - 0 1"
)

// StartingPosition returns the starting position
// ppkn/4/4/NKPP w NCFncf - 0 1
func StartingPosition() (*Position, error) {
	return decodeOFEN(startOFEN)
}

// Update returns a new position resulting from the given move.
// The move itself isn't validated, if validation is needed use
// Game's Move method. This method is more performant for bots that
// rely on the ValidMoves because it skips redundant validation.
func (pos *Position) Update(m *Move) *Position {
	moveCount := pos.moveCount
	if pos.turn == Black {
		moveCount++
	}
	cr := pos.CastleRights()
	ncr := pos.updateCastleRights(m)
	p := pos.board.Piece(m.s1)
	halfMove := pos.halfMoveClock
	if p.Type() == Pawn || isPureCapture(m) || cr != ncr {
		halfMove = 0
	} else {
		halfMove++
	}
	b := pos.board.copy()
	b.update(m)
	return &Position{
		board:           b,
		turn:            pos.turn.Other(),
		castleRights:    ncr,
		enPassantSquare: pos.updateEnPassantSquare(m),
		halfMoveClock:   halfMove,
		moveCount:       moveCount,
		inCheck:         m.HasTag(Check),
	}
}

// ValidMoves returns a list of valid moves for the position.
func (pos *Position) ValidMoves() []*Move {
	if pos.validMoves != nil {
		return append([]*Move(nil), pos.validMoves...)
	}
	pos.validMoves = engine{}.CalcMoves(pos, false)
	return append([]*Move(nil), pos.validMoves...)
}

// Status returns the position's status as one of the outcome methods.
// Possible returns values include Checkmate, Stalemate, and NoMethod.
func (pos *Position) Status() Method {
	return engine{}.Status(pos)
}

// Board returns the position's board.
func (pos *Position) Board() *Board {
	return pos.board
}

// EnPassantSquare returns the position's active en passant square if any.
func (pos *Position) EnPassantSquare() Square {
	return pos.enPassantSquare
}

// Turn returns the color to move next.
func (pos *Position) Turn() Color {
	return pos.turn
}

// CastleRights returns the castling rights of the position.
func (pos *Position) CastleRights() CastleRights {
	return pos.castleRights
}

// InCheck returns true if the king is in check in the position.
func (pos *Position) InCheck() bool {
	return pos.inCheck
}

// CheckSquare returns the square containing the checked king.
func (pos *Position) CheckSquare() Square {
	if pos.inCheck {
		return pos.activeKingSquare()
	}

	return NoSquare
}

// String implements the fmt.Stringer interface and returns a
// string with the OFEN format: ppkn/4/4/NKPP w NCFncf - 0 1
func (pos *Position) String() string {
	b := pos.board.String()
	t := pos.turn.String()
	c := pos.castleRights.String()
	sq := "-"
	if pos.enPassantSquare != NoSquare {
		sq = pos.enPassantSquare.String()
	}
	return fmt.Sprintf("%s %s %s %s %d %d", b, t, c, sq, pos.halfMoveClock, pos.moveCount)
}

// Hash returns a unique hash of the position
func (pos *Position) Hash() [16]byte {
	sq := "-"
	if pos.enPassantSquare != NoSquare {
		sq = pos.enPassantSquare.String()
	}
	s := pos.turn.String() + ":" + pos.castleRights.String() + ":" + sq
	for _, p := range allPieces {
		bb := pos.board.bbForPiece(p)
		s += ":" + strconv.FormatUint(uint64(bb), 16)
	}
	return md5.Sum([]byte(s))
}

// MarshalText implements the encoding.TextMarshaller interface and
// encodes the position's OFEN.
func (pos *Position) MarshalText() (text []byte, err error) {
	return []byte(pos.String()), nil
}

// UnmarshalText implements the encoding.TextUnmarshaler interface and
// assumes the data is in the OFEN format.
func (pos *Position) UnmarshalText(text []byte) error {
	cp, err := decodeOFEN(string(text))
	if err != nil {
		return err
	}
	pos.board = cp.board
	pos.castleRights = cp.castleRights
	pos.turn = cp.turn
	pos.enPassantSquare = cp.enPassantSquare
	pos.halfMoveClock = cp.halfMoveClock
	pos.moveCount = cp.moveCount
	pos.inCheck = isInCheck(cp)
	return nil
}

const (
	bitsCastleWhiteKnight uint8 = 1 << iota
	bitsCastleWhiteClose
	bitsCastleWhiteFar
	bitsCastleBlackKnight
	bitsCastleBlackClose
	bitsCastleBlackFar
	bitsTurn
	bitsHasEnPassant
)

// MarshalBinary implements the encoding.BinaryMarshaller interface
func (pos *Position) MarshalBinary() (data []byte, err error) {
	boardBytes, err := pos.board.MarshalBinary()
	if err != nil {
		return nil, err
	}
	buf := bytes.NewBuffer(boardBytes)
	if err := binary.Write(buf, binary.BigEndian, uint8(pos.halfMoveClock)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, uint16(pos.moveCount)); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, pos.enPassantSquare); err != nil {
		return nil, err
	}
	var b uint8
	if pos.castleRights.CanCastle(White, KnightSide) {
		b = b | bitsCastleWhiteKnight
	}
	if pos.castleRights.CanCastle(White, CloseSide) {
		b = b | bitsCastleWhiteClose
	}
	if pos.castleRights.CanCastle(White, FarSide) {
		b = b | bitsCastleWhiteFar
	}
	if pos.castleRights.CanCastle(Black, KnightSide) {
		b = b | bitsCastleBlackKnight
	}
	if pos.castleRights.CanCastle(Black, CloseSide) {
		b = b | bitsCastleBlackClose
	}
	if pos.castleRights.CanCastle(Black, FarSide) {
		b = b | bitsCastleBlackFar
	}
	if pos.turn == Black {
		b = b | bitsTurn
	}
	if pos.enPassantSquare != NoSquare {
		b = b | bitsHasEnPassant
	}
	if err := binary.Write(buf, binary.BigEndian, b); err != nil {
		return nil, err
	}
	return buf.Bytes(), err
}

// UnmarshalBinary implements the encoding.BinaryMarshaller interface
func (pos *Position) UnmarshalBinary(data []byte) error {
	if len(data) != 29 {
		return errors.New("octad: position binary data should consist of 29 bytes")
	}
	board := &Board{}
	if err := board.UnmarshalBinary(data[:24]); err != nil {
		return err
	}
	pos.board = board

	buf := bytes.NewBuffer(data[24:])

	halfMove := uint8(pos.halfMoveClock)
	if err := binary.Read(buf, binary.BigEndian, &halfMove); err != nil {
		return err
	}
	pos.halfMoveClock = int(halfMove)

	moveCount := uint16(pos.moveCount)
	if err := binary.Read(buf, binary.BigEndian, &moveCount); err != nil {
		return err
	}
	pos.moveCount = int(moveCount)

	if err := binary.Read(buf, binary.BigEndian, &pos.enPassantSquare); err != nil {
		return err
	}

	var b uint8
	if err := binary.Read(buf, binary.BigEndian, &b); err != nil {
		return err
	}

	pos.castleRights = decodeCastleRights(b)

	pos.turn = White
	if b&bitsTurn != 0 {
		pos.turn = Black
	}

	if b&bitsHasEnPassant == 0 {
		pos.enPassantSquare = NoSquare
	}

	pos.inCheck = isInCheck(pos)
	return nil
}

func decodeCastleRights(rights uint8) CastleRights {
	cr := ""
	if rights&bitsCastleWhiteKnight != 0 {
		cr += "N"
	}
	if rights&bitsCastleWhiteClose != 0 {
		cr += "C"
	}
	if rights&bitsCastleWhiteFar != 0 {
		cr += "F"
	}
	if rights&bitsCastleBlackKnight != 0 {
		cr += "n"
	}
	if rights&bitsCastleBlackClose != 0 {
		cr += "c"
	}
	if rights&bitsCastleBlackFar != 0 {
		cr += "f"
	}
	if cr == "" {
		cr = "-"
	}
	return CastleRights(cr)
}

// returns true if the move is not a castle move
func isPureCapture(m *Move) bool {
	return m.HasTag(Capture) && !m.HasTag(FarPawnCastle) &&
		!m.HasTag(ClosePawnCastle) && !m.HasTag(KnightCastle)
}

func (pos *Position) copy() *Position {
	return &Position{
		board:           pos.board.copy(),
		turn:            pos.turn,
		castleRights:    pos.castleRights,
		enPassantSquare: pos.enPassantSquare,
		halfMoveClock:   pos.halfMoveClock,
		moveCount:       pos.moveCount,
		inCheck:         pos.inCheck,
	}
}

func (pos *Position) updateCastleRights(m *Move) CastleRights {
	cr := string(pos.castleRights)

	if didPieceMove(pos, m, WhiteKing, A1) {
		removeCastlingRight(&cr, "N")
	}
	if didPieceMove(pos, m, WhiteKing, C1) {
		removeCastlingRight(&cr, "C")
	}
	if didPieceMove(pos, m, WhiteKing, D1) {
		removeCastlingRight(&cr, "F")
	}
	if didPieceMove(pos, m, BlackKing, D4) {
		removeCastlingRight(&cr, "n")
	}
	if didPieceMove(pos, m, BlackKing, B4) {
		removeCastlingRight(&cr, "c")
	}
	if didPieceMove(pos, m, BlackKing, A4) {
		removeCastlingRight(&cr, "f")
	}
	if cr == "" {
		cr = "-"
	}

	return CastleRights(cr)
}

func didPieceMove(pos *Position, m *Move, p Piece, square Square) bool {
	return pos.board.Piece(m.s1) == p || m.s1 == square || m.s2 == square
}

func removeCastlingRight(rights *string, removedRight string) {
	*rights = strings.Replace(*rights, removedRight, "", -1)
}

func (pos *Position) activeKingSquare() Square {
	kingSq := pos.board.whiteKingSq
	if pos.Turn() == Black {
		kingSq = pos.board.blackKingSq
	}
	return kingSq
}

func (pos *Position) updateEnPassantSquare(m *Move) Square {
	p := pos.board.Piece(m.s1)
	if p.Type() != Pawn {
		return NoSquare
	}
	if pos.turn == White &&
		(bbForSquare(m.s1)&bbRank1) != 0 &&
		(bbForSquare(m.s2)&bbRank3) != 0 {
		return m.s2 - 4
	} else if pos.turn == Black &&
		(bbForSquare(m.s1)&bbRank4) != 0 &&
		(bbForSquare(m.s2)&bbRank2) != 0 {
		return m.s2 + 4
	}
	return NoSquare
}

func (pos *Position) samePosition(pos2 *Position) bool {
	return pos.board.String() == pos2.board.String() &&
		pos.turn == pos2.turn &&
		pos.castleRights.String() == pos2.castleRights.String() &&
		pos.enPassantSquare == pos2.enPassantSquare
}
