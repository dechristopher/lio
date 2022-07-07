package octad

import (
	"math/bits"
	"strconv"
	"strings"
)

// bitboard is an Octad board representation encoded in an unsigned 16-bit
// integer. The 16 board positions begin with A1 as the most significant bit and
// D4 as the least significant.
type bitboard uint16

func newBitboard(m map[Square]bool) bitboard {
	s := ""
	for sq := 0; sq < squaresOnBoard; sq++ {
		if m[Square(sq)] {
			s += "1"
		} else {
			s += "0"
		}
	}
	bb, err := strconv.ParseUint(s, 2, 16)
	if err != nil {
		panic(err)
	}
	return bitboard(bb)
}

func (b bitboard) Mapping() map[Square]bool {
	m := map[Square]bool{}
	for sq := 0; sq < squaresOnBoard; sq++ {
		if b&bbForSquare(Square(sq)) > 0 {
			m[Square(sq)] = true
		}
	}
	return m
}

// String returns a 64 character string of 1s and 0s starting with the most
// significant bit.
func (b bitboard) String() string {
	s := strconv.FormatUint(uint64(b), 2)
	return strings.Repeat("0", squaresOnBoard-len(s)) + s
}

// Draw returns visual representation of the bitboard useful for debugging.
func (b bitboard) Draw() string {
	s := "\n A B C D\n"
	for r := 3; r >= 0; r-- {
		s += Rank(r).String()
		for f := 0; f < squaresInRow; f++ {
			sq := getSquare(File(f), Rank(r))
			if b.Occupied(sq) {
				s += "1"
			} else {
				s += "0"
			}
			s += " "
		}
		s += "\n"
	}
	return s
}

// Reverse returns a bitboard where the bit order is reversed.
func (b bitboard) Reverse() bitboard {
	return bitboard(bits.Reverse16(uint16(b)))
}

// Occupied returns true if the square's bitboard position is 1.
func (b bitboard) Occupied(sq Square) bool {
	return (bits.RotateLeft16(uint16(b), int(sq)+1) & 1) == 1
}
