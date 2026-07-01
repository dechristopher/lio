package engine

import "github.com/dechristopher/octad/v2"

// This file implements the board-aware evaluation features — pawn structure,
// connectivity (defended pieces), and king safety — that the octad library
// does not expose helpers for. Octad's move generator only yields moves to
// empty or enemy squares, so it can't tell us which friendly pieces are
// defended, and its attack routines are unexported. Because the board is only
// 4x4 we implement a small self-contained attack generator here, which lets us
// score these features as proper differentials (computed for both colors)
// rather than side-to-move-only.

// boardDim is the side length of the octad board.
const boardDim = 4

// Move offsets in (file, rank) space.
var (
	knightDirs = [][2]int{{1, 2}, {2, 1}, {2, -1}, {1, -2}, {-1, -2}, {-2, -1}, {-2, 1}, {-1, 2}}
	kingDirs   = [][2]int{{1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}, {0, -1}, {1, -1}}
	diagDirs   = [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}
	orthoDirs  = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
)

// onBoard reports whether the (file, rank) pair lies on the 4x4 board.
func onBoard(f, r int) bool {
	return f >= 0 && f < boardDim && r >= 0 && r < boardDim
}

// sqAt returns the Square at the given file and rank. The caller must ensure
// the coordinates are on the board (Square is indexed rank*4 + file).
func sqAt(f, r int) octad.Square {
	return octad.Square(r*boardDim + f)
}

// pieceAttacks records, into out, every square attacked (and thereby defended)
// by the piece p standing on sq, given the current occupancy. Sliding pieces
// stop at — and include — the first occupied square they encounter.
func pieceAttacks(squares map[octad.Square]octad.Piece, sq octad.Square, p octad.Piece, out map[octad.Square]bool) {
	f, r := int(sq.File()), int(sq.Rank())

	mark := func(dirs [][2]int) {
		for _, d := range dirs {
			if onBoard(f+d[0], r+d[1]) {
				out[sqAt(f+d[0], r+d[1])] = true
			}
		}
	}

	switch p.Type() {
	case octad.Knight:
		mark(knightDirs)
	case octad.King:
		mark(kingDirs)
	case octad.Pawn:
		// pawns attack diagonally forward (white up the board, black down)
		dir := 1
		if p.Color() == octad.Black {
			dir = -1
		}
		mark([][2]int{{-1, dir}, {1, dir}})
	case octad.Bishop:
		slideAttacks(squares, f, r, diagDirs, out)
	case octad.Rook:
		slideAttacks(squares, f, r, orthoDirs, out)
	case octad.Queen:
		slideAttacks(squares, f, r, diagDirs, out)
		slideAttacks(squares, f, r, orthoDirs, out)
	}
}

// slideAttacks walks each ray from (f, r), marking squares until it steps off
// the board or hits an occupied square (which is itself marked as attacked).
func slideAttacks(squares map[octad.Square]octad.Piece, f, r int, dirs [][2]int, out map[octad.Square]bool) {
	for _, d := range dirs {
		nf, nr := f+d[0], r+d[1]
		for onBoard(nf, nr) {
			ns := sqAt(nf, nr)
			out[ns] = true
			if _, occupied := squares[ns]; occupied {
				break
			}
			nf += d[0]
			nr += d[1]
		}
	}
}

// attackedSquares returns the set of squares attacked by all pieces of color.
func attackedSquares(squares map[octad.Square]octad.Piece, color octad.Color) map[octad.Square]bool {
	out := make(map[octad.Square]bool)
	for sq, p := range squares {
		if p.Color() == color {
			pieceAttacks(squares, sq, p, out)
		}
	}
	return out
}

// positionalFeatures returns the side-to-move-relative contribution of pawn
// structure, connectivity, and king safety. Every term is a differential
// (color minus opponent), so it is positive when it favors the side to move.
func positionalFeatures(squares map[octad.Square]octad.Piece, color octad.Color) float64 {
	other := color.Other()
	friendlyAttacks := attackedSquares(squares, color)
	enemyAttacks := attackedSquares(squares, other)

	eval := pawnStructureScore(squares, color) - pawnStructureScore(squares, other)

	// connectivity: own non-king pieces defended by a friendly piece
	eval += ConnectivityWeight *
		float64(defendedCount(squares, color, friendlyAttacks)-
			defendedCount(squares, other, enemyAttacks))

	// king safety: safe squares each king could flee to
	eval += KingSafetyWeight *
		float64(kingEscapes(squares, color, enemyAttacks)-
			kingEscapes(squares, other, friendlyAttacks))

	return eval
}

// pawnStructureScore scores color's pawn structure: penalties for doubled and
// isolated pawns, a bonus for passed pawns.
func pawnStructureScore(squares map[octad.Square]octad.Piece, color octad.Color) float64 {
	var fileCount [boardDim]int
	type pawn struct{ f, r int }
	var pawns []pawn
	var enemyPawn [boardDim][boardDim]bool

	for sq, p := range squares {
		if p.Type() != octad.Pawn {
			continue
		}
		f, r := int(sq.File()), int(sq.Rank())
		if p.Color() == color {
			fileCount[f]++
			pawns = append(pawns, pawn{f, r})
		} else {
			enemyPawn[f][r] = true
		}
	}

	// direction this color's pawns advance toward promotion
	forward := 1
	if color == octad.Black {
		forward = -1
	}

	score := 0.0
	for _, pw := range pawns {
		// doubled: shares its file with another friendly pawn
		if fileCount[pw.f] > 1 {
			score -= DoubledPawnPenalty
		}

		// isolated: no friendly pawn on either adjacent file
		isolated := true
		if pw.f-1 >= 0 && fileCount[pw.f-1] > 0 {
			isolated = false
		}
		if pw.f+1 < boardDim && fileCount[pw.f+1] > 0 {
			isolated = false
		}
		if isolated {
			score -= IsolatedPawnPenalty
		}

		// passed: no enemy pawn ahead on its own or an adjacent file
		passed := true
		for nf := pw.f - 1; nf <= pw.f+1; nf++ {
			if nf < 0 || nf >= boardDim {
				continue
			}
			for nr := pw.r + forward; nr >= 0 && nr < boardDim; nr += forward {
				if enemyPawn[nf][nr] {
					passed = false
				}
			}
		}
		if passed {
			score += PassedPawnBonus
		}
	}

	return score
}

// defendedCount returns how many of color's non-king pieces stand on a square
// that one of their own pieces attacks (i.e. could recapture on).
func defendedCount(squares map[octad.Square]octad.Piece, color octad.Color, friendlyAttacks map[octad.Square]bool) int {
	n := 0
	for sq, p := range squares {
		if p.Color() != color || p.Type() == octad.King {
			continue
		}
		if friendlyAttacks[sq] {
			n++
		}
	}
	return n
}

// kingEscapes counts the squares adjacent to color's king that it could legally
// flee to: on the board, not occupied by a friendly piece, and not attacked by
// the enemy.
func kingEscapes(squares map[octad.Square]octad.Piece, color octad.Color, enemyAttacks map[octad.Square]bool) int {
	kingSq := octad.NoSquare
	for sq, p := range squares {
		if p.Color() == color && p.Type() == octad.King {
			kingSq = sq
			break
		}
	}
	if kingSq == octad.NoSquare {
		return 0
	}

	f, r := int(kingSq.File()), int(kingSq.Rank())
	n := 0
	for _, d := range kingDirs {
		nf, nr := f+d[0], r+d[1]
		if !onBoard(nf, nr) {
			continue
		}
		ns := sqAt(nf, nr)
		if p, occupied := squares[ns]; occupied && p.Color() == color {
			continue
		}
		if enemyAttacks[ns] {
			continue
		}
		n++
	}
	return n
}
