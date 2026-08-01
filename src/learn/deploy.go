package learn

import (
	"strings"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/engine"
)

// The deploy lesson teaches the blind arrangement phase: both players privately
// order their four pieces along their own home rank, the setups are revealed
// together, and the game begins from whatever that produced.
//
// The mirroring here is the same contract room/deploy.go implements for real
// games — each side arranges from its own left to right, so identical orderings
// reproduce the symmetric standard position — but this package deliberately
// does not import room. A lesson must not drag a live game's state machine,
// persistence and clocks in behind it just to lay out four pieces, so the rule
// is restated in the few lines it takes. TestDeployMirrorsStandard pins the two
// together at the position everybody can check by eye: the classic start.

// deployArmy is the legal Octad army every arrangement must be a permutation
// of: one king, one knight, two pawns.
const deployArmy = "knpp"

// parseArrangement decodes four piece letters (k/n/p, case-insensitive) given
// from the player's own left to right, rejecting anything that is not the legal
// army.
func parseArrangement(order string) ([4]octad.PieceType, bool) {
	var out [4]octad.PieceType
	if len(order) != 4 {
		return out, false
	}
	var kings, knights, pawns int
	for i, ch := range strings.ToLower(order) {
		switch ch {
		case 'k':
			out[i] = octad.King
			kings++
		case 'n':
			out[i] = octad.Knight
			knights++
		case 'p':
			out[i] = octad.Pawn
			pawns++
		default:
			return out, false
		}
	}
	if kings != 1 || knights != 1 || pawns != 2 {
		return out, false
	}
	return out, true
}

// ofenChar is a deployable piece's OFEN letter for the given color.
func ofenChar(pt octad.PieceType, c octad.Color) string {
	var s string
	switch pt {
	case octad.King:
		s = "K"
	case octad.Knight:
		s = "N"
	case octad.Pawn:
		s = "P"
	default:
		return ""
	}
	if c == octad.Black {
		return strings.ToLower(s)
	}
	return s
}

// assembleOFEN builds the starting position from both arrangements. White is
// given from the learner's own perspective, which for White already runs a..d
// left to right; black is given in board order (files a..d), which is how the
// engine hands back a placement. All castle rights are granted — nothing has
// moved yet — and White is to move.
func assembleOFEN(white [4]octad.PieceType, black [4]octad.PieceType) (string, bool) {
	var rank1, rank4 strings.Builder
	for i := 0; i < 4; i++ {
		rank1.WriteString(ofenChar(white[i], octad.White))
		rank4.WriteString(ofenChar(black[i], octad.Black))
	}
	ofen := rank4.String() + "/4/4/" + rank1.String() + " w NCFncf - 0 1"
	if _, err := octad.OFEN(ofen); err != nil {
		return "", false
	}
	return ofen, true
}

// doDeploy commits the learner's arrangement and reveals the opponent's,
// answering with the position the game starts from. The opponent arranges the
// way the tutorial's own bot does — the weakest persona deploys at random —
// so the reveal shows a genuinely different setup nearly every time, which is
// the whole point of the lesson.
func doDeploy(step Step, req Request) (*Response, error) {
	white, ok := parseArrangement(req.Deploy)
	if !ok {
		return nil, ErrBadRequest
	}
	black := engine.RandomDeployment()
	ofen, ok := assembleOFEN(white, black)
	if !ok {
		return nil, ErrBadRequest
	}
	g, err := newGame(ofen)
	if err != nil {
		return nil, ErrBadRequest
	}

	resp := &Response{Done: true, Say: step.Success}
	describe(resp, g)
	return resp, nil
}
