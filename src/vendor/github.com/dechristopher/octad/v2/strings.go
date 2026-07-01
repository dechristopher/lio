package octad

import "fmt"

// String implements the fmt.Stringer interface for Method, returning the name
// of the method that produced a game's outcome.
func (i Method) String() string {
	switch i {
	case NoMethod:
		return "NoMethod"
	case Checkmate:
		return "Checkmate"
	case Resignation:
		return "Resignation"
	case DrawOffer:
		return "DrawOffer"
	case Stalemate:
		return "Stalemate"
	case ThreefoldRepetition:
		return "ThreefoldRepetition"
	case TwentyFiveMoveRule:
		return "TwentyFiveMoveRule"
	case InsufficientMaterial:
		return "InsufficientMaterial"
	}
	return fmt.Sprintf("Method(%d)", int(i))
}
