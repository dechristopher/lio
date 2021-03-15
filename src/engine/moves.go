package engine

import "github.com/dechristopher/octad"

// orderMoves attempts to reorder the generated moves
// list to give priority to higher impact moves that
// may evaluate similarly to lower impact moves but
// end up being more decisive in the grand scheme
func orderMoves(situation *octad.Game) []octad.Move {
	ordered := make([]octad.Move, 0)
	var movesLeft []octad.Move

	for _, m := range situation.ValidMoves() {
		movesLeft = append(movesLeft, *m)
	}

	var checks []octad.Move
	var captures []octad.Move
	var promotions []octad.Move
	var queen []octad.Move
	var rbn []octad.Move
	var castles []octad.Move
	var king []octad.Move
	var other []octad.Move

	for _, m := range movesLeft {
		// add checks
		if m.HasTag(octad.Check) {
			checks = append(checks, m)
			continue
		}

		// add captures
		if m.HasTag(octad.Capture) {
			captures = append(captures, m)
			continue
		}

		// add promotions
		if m.Promo() != octad.NoPieceType {
			promotions = append(promotions, m)
			continue
		}

		piece := situation.Position().Board().Piece(m.S1()).Type()

		// add queen moves
		if piece == octad.Queen {
			queen = append(queen, m)
			continue
		}

		// add rook, bishop, knight moves
		if piece == octad.Rook || piece == octad.Bishop || piece == octad.Knight {
			rbn = append(rbn, m)
			continue
		}

		// add castles
		if m.HasTag(octad.KnightCastle) ||
			m.HasTag(octad.ClosePawnCastle) ||
			m.HasTag(octad.FarPawnCastle) {
			castles = append(castles, m)
			continue
		}

		// add king moves
		if piece == octad.King {
			king = append(king, m)
			continue
		}

		other = append(other, m)
	}

	ordered = append(ordered, checks...)
	ordered = append(ordered, captures...)
	ordered = append(ordered, promotions...)
	ordered = append(ordered, queen...)
	ordered = append(ordered, rbn...)
	ordered = append(ordered, castles...)
	ordered = append(ordered, king...)
	ordered = append(ordered, other...)

	return ordered
}
