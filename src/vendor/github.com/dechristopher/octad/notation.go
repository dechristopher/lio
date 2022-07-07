package octad

import (
	"fmt"
	"strings"
)

// Encoder is the interface implemented by objects that can
// encode a move into a string given the position. It is not
// the encoders responsibility to validate the move.
type Encoder interface {
	Encode(pos *Position, m *Move) string
}

// Decoder is the interface implemented by objects that can
// decode a string into a move given the position. It is not
// the decoders responsibility to validate the move. An error
// is returned if the string could not be decoded.
type Decoder interface {
	Decode(pos *Position, s string) (*Move, error)
}

// Notation is the interface implemented by objects that can
// encode and decode moves.
type Notation interface {
	Encoder
	Decoder
}

// UOINotation is a more computer friendly alternative to algebraic
// notation. This notation uses the same format as the UOI (Universal Octad
// Interface). Example: d3d4q (for promotion)
type UOINotation struct{}

// String implements the fmt.Stringer interface and returns
// the notation's name.
func (UOINotation) String() string {
	return "UCI Notation"
}

// Encode implements the Encoder interface.
func (UOINotation) Encode(pos *Position, m *Move) string {
	return m.S1().String() + m.S2().String() + m.Promo().String()
}

// Decode implements the Decoder interface.
func (UOINotation) Decode(pos *Position, s string) (*Move, error) {
	l := len(s)
	err := fmt.Errorf(`octad: failed to decode long algebraic notation text "%s" for position %s`, s, pos)
	if l < 4 || l > 5 {
		return nil, err
	}
	s1, ok := strToSquareMap[s[0:2]]
	if !ok {
		return nil, err
	}
	s2, ok := strToSquareMap[s[2:4]]
	if !ok {
		return nil, err
	}
	promo := NoPieceType
	if l == 5 {
		promo = pieceTypeFromChar(s[4:5])
		if promo == NoPieceType {
			return nil, err
		}
	}
	m := &Move{s1: s1, s2: s2, promo: promo}
	if pos == nil {
		return m, nil
	}
	p := pos.Board().Piece(s1)
	p2 := pos.Board().Piece(s2)

	if p.Type() == King {
		if ((s1 == B1 && s2 == A1) || (s1 == C4 && s2 == D4)) && p2.Type() == Knight && p.Color() == p2.Color() {
			m.addTag(KnightCastle)
		} else if ((s1 == B1 && s2 == C1) || (s1 == C4 && s2 == B4)) && p2.Type() == Pawn && p.Color() == p2.Color() {
			m.addTag(ClosePawnCastle)
		} else if ((s1 == B1 && s2 == D1) || (s1 == C4 && s2 == A4)) && p2.Type() == Pawn && p.Color() == p2.Color() {
			m.addTag(FarPawnCastle)
		}
	} else if p.Type() == Pawn && s2 == pos.enPassantSquare {
		m.addTag(EnPassant)
		m.addTag(Capture)
	}

	c1 := p.Color()
	c2 := pos.Board().Piece(s2).Color()
	if c2 != NoColor && c1 != c2 {
		m.addTag(Capture)
	}
	return m, nil
}

// AlgebraicNotation (or Standard Algebraic Notation) is the
// official octad notation used by FIDE. Examples: c2, b3,
// O-O (short castling), d4=Q (promotion)
type AlgebraicNotation struct{}

// String implements the fmt.Stringer interface and returns
// the notation's name.
func (AlgebraicNotation) String() string {
	return "Algebraic Notation"
}

// Encode implements the Encoder interface.
func (AlgebraicNotation) Encode(pos *Position, m *Move) string {
	checkChar := getCheckChar(pos, m)
	if m.HasTag(KnightCastle) {
		return "O" + checkChar
	} else if m.HasTag(ClosePawnCastle) {
		return "O-O" + checkChar
	} else if m.HasTag(FarPawnCastle) {
		return "O-O-O" + checkChar
	}
	p := pos.Board().Piece(m.S1())
	pChar := charFromPieceType(p.Type())
	s1Str := formS1(pos, m)
	capChar := ""
	if m.HasTag(Capture) || m.HasTag(EnPassant) {
		capChar = "x"
		if p.Type() == Pawn && s1Str == "" {
			capChar = m.s1.File().String() + "x"
		}
	}
	promoText := charForPromo(m.promo)
	return pChar + s1Str + capChar + m.s2.String() + promoText + checkChar
}

// Decode implements the Decoder interface.
func (AlgebraicNotation) Decode(pos *Position, s string) (*Move, error) {
	s = removeSubstrings(s, "?", "!", "+", "#", "e.p.")
	for _, m := range pos.ValidMoves() {
		str := AlgebraicNotation{}.Encode(pos, m)
		str = removeSubstrings(str, "?", "!", "+", "#", "e.p.")
		if str == s {
			return m, nil
		}
	}
	return nil, fmt.Errorf("octad: could not decode algebraic notation %s for position %s", s, pos.String())
}

// LongAlgebraicNotation is a fully expanded version of
// algebraic notation in which the starting and ending
// squares are specified.
// Examples: e2e4, Rd3xd7, O-O (short castling), e7e8=Q (promotion)
type LongAlgebraicNotation struct{}

// String implements the fmt.Stringer interface and returns
// the notation's name.
func (LongAlgebraicNotation) String() string {
	return "Long Algebraic Notation"
}

// Encode implements the Encoder interface.
func (LongAlgebraicNotation) Encode(pos *Position, m *Move) string {
	checkChar := getCheckChar(pos, m)
	if m.HasTag(KnightCastle) {
		return "O" + checkChar
	} else if m.HasTag(ClosePawnCastle) {
		return "O-O" + checkChar
	} else if m.HasTag(FarPawnCastle) {
		return "O-O-O" + checkChar
	}
	p := pos.Board().Piece(m.S1())
	pChar := charFromPieceType(p.Type())
	s1Str := m.s1.String()
	capChar := ""
	if m.HasTag(Capture) || m.HasTag(EnPassant) {
		capChar = "x"
		if p.Type() == Pawn && s1Str == "" {
			capChar = m.s1.File().String() + "x"
		}
	}
	promoText := charForPromo(m.promo)
	return pChar + s1Str + capChar + m.s2.String() + promoText + checkChar
}

// Decode implements the Decoder interface.
func (LongAlgebraicNotation) Decode(pos *Position, s string) (*Move, error) {
	s = removeSubstrings(s, "?", "!", "+", "#", "e.p.")
	for _, m := range pos.ValidMoves() {
		str := LongAlgebraicNotation{}.Encode(pos, m)
		str = removeSubstrings(str, "?", "!", "+", "#", "e.p.")
		if str == s {
			return m, nil
		}
	}
	return nil, fmt.Errorf("octad: could not decode long algebraic notation %s for position %s", s, pos.String())
}

func getCheckChar(pos *Position, move *Move) string {
	if !move.HasTag(Check) {
		return ""
	}
	nextPos := pos.Update(move)
	if nextPos.Status() == Checkmate {
		return "#"
	}
	return "+"
}

func formS1(pos *Position, m *Move) string {
	p := pos.board.Piece(m.s1)
	if p.Type() == Pawn {
		return ""
	}

	var req, fileReq, rankReq bool
	moves := pos.ValidMoves()

	for _, mv := range moves {
		if mv.s1 != m.s1 && mv.s2 == m.s2 && p == pos.board.Piece(mv.s1) {
			req = true

			if mv.s1.File() == m.s1.File() {
				rankReq = true
			}

			if mv.s1.Rank() == m.s1.Rank() {
				fileReq = true
			}
		}
	}

	var s1 = ""

	if fileReq || !rankReq && req {
		s1 = m.s1.File().String()
	}

	if rankReq {
		s1 += m.s1.Rank().String()
	}

	return s1
}

func charForPromo(p PieceType) string {
	c := charFromPieceType(p)
	if c != "" {
		c = "=" + c
	}
	return c
}

func charFromPieceType(p PieceType) string {
	switch p {
	case King:
		return "K"
	case Queen:
		return "Q"
	case Rook:
		return "R"
	case Bishop:
		return "B"
	case Knight:
		return "N"
	}
	return ""
}

func pieceTypeFromChar(c string) PieceType {
	switch c {
	case "q":
		return Queen
	case "r":
		return Rook
	case "b":
		return Bishop
	case "n":
		return Knight
	}
	return NoPieceType
}

func removeSubstrings(s string, subs ...string) string {
	for _, sub := range subs {
		s = strings.Replace(s, sub, "", -1)
	}
	return s
}
