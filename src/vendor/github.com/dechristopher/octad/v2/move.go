package octad

// A MoveTag represents a notable consequence of a move.
type MoveTag uint16

const (
	// NearCastle indicates that the move is a near castle (the king swapping
	// with an adjacent knight).
	NearCastle MoveTag = 1 << iota
	// CenterCastle indicates that the move is a center castle (the king
	// swapping with an adjacent pawn).
	CenterCastle
	// FarCastle indicates that the move is a far castle (the king leaping one
	// square over an empty gap to swap with a pawn two files away).
	FarCastle
	// Capture indicates that the move captures a piece.
	Capture
	// EnPassant indicates that the move captures via en passant.
	EnPassant
	// Check indicates that the move puts the opposing player in check.
	Check
	// inCheck indicates that the move puts the moving player in check and
	// is therefore invalid.
	inCheck
)

// A Move is the movement of a piece from one square to another.
type Move struct {
	s1    Square
	s2    Square
	promo PieceType
	tags  MoveTag
}

// String returns a string useful for debugging. String doesn't return
// algebraic notation.
func (m *Move) String() string {
	return m.s1.String() + m.s2.String() + m.promo.String()
}

// Equals returns whether or not the move in question is exactly the
// same as the current move
func (m *Move) Equals(move *Move) bool {
	return m.String() == move.String() &&
		m.tags == move.tags &&
		m.promo == move.promo
}

// S1 returns the origin square of the move.
func (m *Move) S1() Square {
	return m.s1
}

// S2 returns the destination square of the move.
func (m *Move) S2() Square {
	return m.s2
}

// Promo returns promotion piece type of the move.
func (m *Move) Promo() PieceType {
	return m.promo
}

// HasTag returns true if the move contains the MoveTag given.
func (m *Move) HasTag(tag MoveTag) bool {
	return (tag & m.tags) > 0
}

func (m *Move) addTag(tag MoveTag) {
	m.tags = m.tags | tag
}

type moveSlice []*Move

func (a moveSlice) find(m *Move) *Move {
	if m == nil {
		return nil
	}
	for _, move := range a {
		if m.Equals(move) {
			return move
		}
	}
	return nil
}
