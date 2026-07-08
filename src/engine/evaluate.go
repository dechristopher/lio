package engine

import (
	"github.com/dechristopher/octad/v2"
)

/*
 * TODO
 * - board evaluation function
 *   X piece value
 *   X pawn location (rank handled via piece-square tables)
 *      X pawn structure (doubled / isolated / passed)
 *      X rank (piece-square tables)
 *   X mobility (num legal moves)
 *   X can promote
 *   X can castle
 *   X connectivity (pieces defended)
 *   X king
 *     X king safety (in-check penalty + safe king escape squares)
 *
 * X basic minimax search
 *   X alpha-beta pruning
 *   X depth limiting for capping rating
 */

type materialValues = map[octad.Color]float64

const WinVal float64 = 10000

// PieceVals contains the material evaluation value
// of each piece type in octad
var PieceVals = map[octad.PieceType]float64{
	octad.King:        1000,
	octad.Queen:       90,
	octad.Rook:        50,
	octad.Bishop:      31,
	octad.Knight:      30,
	octad.Pawn:        10,
	octad.NoPieceType: 0,
}

// Positional term weights. These are intentionally small relative to material
// (a pawn is worth 10) so that piece count remains the dominant signal; they
// are tunable knobs for engine strength, not correctness.
const (
	// CheckPenalty is applied to the side to move when it is in check: a
	// minor positional liability (restricted mobility, forced response).
	CheckPenalty float64 = 25
	// MobilityWeight rewards the side to move per legal move available.
	MobilityWeight float64 = 0.5
	// PromoWeight rewards the side to move per pawn that can legally promote
	// this move.
	PromoWeight float64 = 5
	// CastleWeight rewards each castling right retained, relative to the
	// opponent, to value development flexibility.
	CastleWeight float64 = 2
	// DoubledPawnPenalty is charged per pawn sharing its file with a friendly pawn.
	DoubledPawnPenalty float64 = 4
	// IsolatedPawnPenalty is charged per pawn with no friendly pawn on an adjacent file.
	IsolatedPawnPenalty float64 = 5
	// PassedPawnBonus rewards a pawn with no enemy pawn ahead on its own or adjacent files.
	PassedPawnBonus float64 = 6
	// ConnectivityWeight rewards each own (non-king) piece defended by a friendly piece.
	ConnectivityWeight float64 = 2
	// KingSafetyWeight rewards each safe square the king can flee to.
	KingSafetyWeight float64 = 3
	// MopUpCenterWeight rewards, per square of center distance, having pushed
	// a bare enemy king toward the board edge/corner where it can be mated
	// (see mopUpTerm). Only active when the opponent has just a king left.
	MopUpCenterWeight float64 = 6
	// MopUpProximityWeight rewards the winning king closing the Manhattan
	// distance to the bare enemy king; mates need the kings near each other.
	MopUpProximityWeight float64 = 2
)

// castleSides enumerates octad's three castling options.
var castleSides = []octad.Side{octad.NearSide, octad.CenterSide, octad.FarSide}

// castleRightsCount returns how many of the three octad castling options the
// given color still retains in the position.
func castleRightsCount(situation *octad.Game, c octad.Color) int {
	cr := situation.Position().CastleRights()
	n := 0
	for _, side := range castleSides {
		if cr.CanCastle(c, side) {
			n++
		}
	}
	return n
}

// Evaluate returns a numerical evaluation of a game situation relative to
// the side to move: positive means the player whose turn it is is winning,
// negative means they are losing, and zero is a completely drawn game. The
// minimax search relies on this side-to-move-relative convention (see the
// sign flip in mmABMin / the color multiplier in negamax).
func Evaluate(situation *octad.Game) float64 {
	color := situation.Position().Turn()

	switch situation.Outcome() {
	case octad.WhiteWon:
		if color == octad.White {
			return WinVal
		}
		return -WinVal
	case octad.BlackWon:
		if color == octad.White {
			return -WinVal
		}
		return WinVal
	case octad.Draw:
		return 0.0
	default: // continue evaluation if no outcome
		break
	}

	eval := staticEval(situation, color)

	// InCheck always refers to the side to move, and being in check is bad
	// for the side to move, so always penalize regardless of color. (The
	// previous code rewarded black for being in check by flipping the sign,
	// which made the engine avoid giving checks and shed material to do so.)
	if situation.Position().InCheck() {
		eval -= CheckPenalty
	}

	return eval
}

// staticEval is the non-terminal, non-check portion of Evaluate: material,
// piece-square tables, mobility, promotion threat, castling rights, and the
// pawn-structure / connectivity / king-safety features. It is kept separate so
// the check term can be verified in isolation. The result is relative to the
// side to move (color), so each term is built as (us - them) or rewards the
// mover directly.
func staticEval(situation *octad.Game, color octad.Color) float64 {
	squareMap := situation.Position().Board().SquareMap()

	// calculate material values and piece position values
	material := make(materialValues)
	posValues := make(materialValues)
	for square, piece := range squareMap {
		material[piece.Color()] += PieceVals[piece.Type()]
		// calc piece position values for pieces with square tables
		if PieceSquareTables[piece.Color()][piece.Type()] != nil {
			posValues[piece.Color()] +=
				PieceSquareTables[piece.Color()][piece.Type()][square]
		}
	}

	// material difference
	eval := material[color] - material[color.Other()]

	// positional value difference
	eval += posValues[color] - posValues[color.Other()]

	// the position is non-terminal here, so the side to move has moves
	moves := situation.ValidMoves()

	// mobility: reward the side to move for having more legal options. Only
	// the mover's move list is cheaply available, but at a fixed search depth
	// every non-terminal leaf shares the same side to move, so this stays a
	// consistent signal across the search tree.
	eval += MobilityWeight * float64(len(moves))

	// promotion threat: reward having a pawn that can legally promote right
	// now. Dedupe by origin square, so a pawn with several promotion choices
	// (queen, rook, ...) is only counted once.
	promoters := make(map[octad.Square]bool)
	for _, m := range moves {
		if m.Promo() != octad.NoPieceType {
			promoters[m.S1()] = true
		}
	}
	eval += PromoWeight * float64(len(promoters))

	// castling rights: reward retaining the flexibility to castle, relative
	// to the opponent
	eval += CastleWeight *
		float64(castleRightsCount(situation, color)-castleRightsCount(situation, color.Other()))

	// pawn structure, connectivity, and king safety (all differential)
	eval += positionalFeatures(squareMap, color)

	// mop-up: with a bare enemy king, reward driving it to the edge and
	// closing in with our own king, so a won endgame has a progress gradient
	// instead of an eval-flat shuffle into the threefold-repetition draw
	eval += mopUpTerm(squareMap, color)

	return eval
}
