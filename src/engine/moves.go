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

	var tempMoves []octad.Move

	// add checks
	for _, m := range movesLeft {
		if m.HasTag(octad.Check) {
			ordered = append(ordered, m)
			continue
		}
		tempMoves = append(tempMoves, m)
	}
	movesLeft = tempMoves
	tempMoves = []octad.Move{}

	// add captures
	for _, m := range movesLeft {
		if m.HasTag(octad.Capture) {
			ordered = append(ordered, m)
			continue
		}
		tempMoves = append(tempMoves, m)
	}
	movesLeft = tempMoves
	tempMoves = []octad.Move{}

	// add promotions
	for _, m := range movesLeft {
		if m.Promo() != octad.NoPieceType {
			ordered = append(ordered, m)
			continue
		}
		tempMoves = append(tempMoves, m)
	}
	movesLeft = tempMoves
	tempMoves = []octad.Move{}

	// add queen moves
	for _, m := range movesLeft {
		piece := situation.Position().Board().Piece(m.S1()).Type()
		if piece == octad.Queen {
			ordered = append(ordered, m)
			continue
		}
		tempMoves = append(tempMoves, m)
	}
	movesLeft = tempMoves
	tempMoves = []octad.Move{}

	// add rook, bishop, knight moves
	for _, m := range movesLeft {
		piece := situation.Position().Board().Piece(m.S1()).Type()
		if piece == octad.Rook || piece == octad.Bishop || piece == octad.Knight {
			ordered = append(ordered, m)
			continue
		}
		tempMoves = append(tempMoves, m)
	}
	movesLeft = tempMoves
	tempMoves = []octad.Move{}

	// add castles
	for _, m := range movesLeft {
		if m.HasTag(octad.KnightCastle) ||
			m.HasTag(octad.ClosePawnCastle) ||
			m.HasTag(octad.FarPawnCastle) {
			ordered = append(ordered, m)
			continue
		}
		tempMoves = append(tempMoves, m)
	}
	movesLeft = tempMoves
	tempMoves = []octad.Move{}

	// add king moves
	for _, m := range movesLeft {
		piece := situation.Position().Board().Piece(m.S1()).Type()
		if piece == octad.King {
			ordered = append(ordered, m)
			continue
		}
		tempMoves = append(tempMoves, m)
	}
	movesLeft = tempMoves
	tempMoves = []octad.Move{}

	// add all other moves
	for _, m := range movesLeft {
		ordered = append(ordered, m)
	}

	return ordered
}
