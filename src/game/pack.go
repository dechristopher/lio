package game

import "github.com/dechristopher/octad/v2"

// Move packing for the durable game archive (arch/STATE_PERSISTENCE_SCALING.md,
// Layer 3). octad's 4x4 board (16 squares, index 0-15) lets an entire move fit
// in two bytes: origin square in bits 0-3, destination square in bits 4-7, and
// the promotion piece type (0-6) in bits 8-10. A finished game's move list is
// then a packed []int16 (~1 byte/ply after Postgres BYTEA storage), and the
// per-move analytics rows carry the same compact SMALLINT.

// PackMove encodes an octad move into a compact int16. See UnpackMoveUOI for
// the inverse; UnpackMoveUOI(PackMove(m)) == m.String() for every legal move.
func PackMove(m *octad.Move) int16 {
	return int16(m.S1()&0xF) | int16(m.S2()&0xF)<<4 | int16(m.Promo()&0x7)<<8
}

// UnpackMoveUOI decodes a PackMove value back into its UOI string (origin +
// destination + optional promotion), matching octad.Move.String().
func UnpackMoveUOI(mv int16) string {
	s1 := octad.Square(mv & 0xF)
	s2 := octad.Square((mv >> 4) & 0xF)
	promo := octad.PieceType((mv >> 8) & 0x7)
	return s1.String() + s2.String() + promo.String()
}
