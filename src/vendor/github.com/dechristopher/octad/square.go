package octad

const (
	squaresOnBoard = 16
	squaresInRow   = 4
)

// A Square is one of the 16 rank and file combinations that make up a board.
type Square int8

// File returns the square's file.
func (sq Square) File() File {
	return File(int(sq) % squaresInRow)
}

// Rank returns the square's rank.
func (sq Square) Rank() Rank {
	return Rank(int(sq) / squaresInRow)
}

// String returns the string representation of the current square
func (sq Square) String() string {
	return sq.File().String() + sq.Rank().String()
}

// Color returns the color of a given square
func (sq Square) Color() Color {
	if ((sq / 4) % 2) == (sq % 2) {
		return Black
	}
	return White
}

func getSquare(f File, r Rank) Square {
	return Square((int(r) * 4) + int(f))
}

const (
	// NoSquare represents an invalid square
	NoSquare Square = iota - 1
	// A1 square, index 0
	A1
	// B1 square, index 1
	B1
	// C1 square, index 2
	C1
	// D1 square, index 3
	D1
	// A2 square, index 4
	A2
	// B2 square, index 5
	B2
	// C2 square, index 6
	C2
	// D2 square, index 7
	D2
	// A3 square, index 8
	A3
	// B3 square, index 9
	B3
	// C3 square, index 10
	C3
	// D3 square, index 11
	D3
	// A4 square, index 12
	A4
	// B4 square, index 13
	B4
	// C4 square, index 14
	C4
	// D4 square, index 15
	D4
)

const (
	fileChars = "abcd"
	rankChars = "1234"
)

// A Rank is the rank of a square.
type Rank int8

const (
	// Rank1 is the first rank, index 0
	Rank1 Rank = iota
	// Rank2 is the second rank, index 1
	Rank2
	// Rank3 is the third rank, index 2
	Rank3
	// Rank4 is the fourth rank, index 3
	Rank4
)

// String returns the string representation of the current rank
func (r Rank) String() string {
	return rankChars[r : r+1]
}

// A File is the file of a square.
type File int8

const (
	// FileA is the A file, index 0
	FileA File = iota
	// FileB is the B file, index 1
	FileB
	// FileC is the C file, index 2
	FileC
	// FileD is the D file, index 3
	FileD
)

// String returns the string representation of the the current file
func (f File) String() string {
	return fileChars[f : f+1]
}

var (
	strToSquareMap = map[string]Square{
		"a1": A1, "a2": A2, "a3": A3, "a4": A4,
		"b1": B1, "b2": B2, "b3": B3, "b4": B4,
		"c1": C1, "c2": C2, "c3": C3, "c4": C4,
		"d1": D1, "d2": D2, "d3": D3, "d4": D4,
	}
)
