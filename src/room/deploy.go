package room

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/dechristopher/octad/v2"
)

// deployTimeout bounds the blind deploy phase. When it expires, any player who
// has not submitted has their arrangement auto-filled with the standard
// ordering so the game always begins. It is a var so tests can shorten it.
var deployTimeout = 30 * time.Second

// Deployment is a player's blind home-rank arrangement of their four pieces
// (one king, one knight, two pawns), given from that player's own left-to-right
// perspective: index 0 is the player's leftmost home-rank square.
type Deployment [4]octad.PieceType

// standardDeployment is the classic Octad ordering (knight, king, pawn, pawn)
// from a player's perspective — the layout of the non-deploy starting position.
// It is the deterministic auto-fill for a player who never submits.
var standardDeployment = Deployment{octad.Knight, octad.King, octad.Pawn, octad.Pawn}

// valid reports whether the deployment is exactly one king, one knight, and two
// pawns — the legal Octad army.
func (d Deployment) valid() bool {
	var kings, knights, pawns int
	for _, pt := range d {
		switch pt {
		case octad.King:
			kings++
		case octad.Knight:
			knights++
		case octad.Pawn:
			pawns++
		default:
			return false
		}
	}
	return kings == 1 && knights == 1 && pawns == 2
}

// order renders the deployment back into the 4-character k/n/p order string the
// client protocol uses (player's own left-to-right perspective) — the inverse of
// parseDeployment. Used to replay a player's committed arrangement on reconnect.
func (d Deployment) order() string {
	var b strings.Builder
	for _, pt := range d {
		switch pt {
		case octad.King:
			b.WriteByte('k')
		case octad.Knight:
			b.WriteByte('n')
		case octad.Pawn:
			b.WriteByte('p')
		}
	}
	return b.String()
}

// randomDeployment returns a uniformly random legal arrangement. The bot now
// deploys via the engine (see requestEngineDeploy / engine.SelectDeployment);
// this remains as a deterministic-random fallback and is exercised by tests.
func randomDeployment() Deployment {
	d := standardDeployment
	rand.Shuffle(len(d), func(i, j int) { d[i], d[j] = d[j], d[i] })
	return d
}

// deploymentFromPlacement converts an engine board-order placement (index i =
// file a+i on the color's home rank) into a player-perspective Deployment (index
// 0 = that player's leftmost home square). White's own perspective already runs
// a..d left to right, so it is a straight copy; black's board is flipped in the
// client, so its perspective is the board order reversed. This is the exact
// inverse of assembleDeployedOFEN's per-side mirroring, so the game the room
// later assembles from the returned Deployment reproduces the position the
// engine actually scored (guarded by TestDeploymentFromPlacementRoundTrip).
func deploymentFromPlacement(color octad.Color, p [4]octad.PieceType) Deployment {
	var d Deployment
	for i := 0; i < 4; i++ {
		if color == octad.Black {
			d[i] = p[3-i]
		} else {
			d[i] = p[i]
		}
	}
	return d
}

// parseDeployment decodes a 4-character order string of piece-type letters
// (k/n/p, case-insensitive) from the player's perspective into a Deployment. An
// error is returned if the string is malformed or is not a legal army.
func parseDeployment(order string) (Deployment, error) {
	if len(order) != 4 {
		return Deployment{}, fmt.Errorf("room: deployment order must be 4 chars, got %q", order)
	}
	var d Deployment
	for i, ch := range strings.ToLower(order) {
		switch ch {
		case 'k':
			d[i] = octad.King
		case 'n':
			d[i] = octad.Knight
		case 'p':
			d[i] = octad.Pawn
		default:
			return Deployment{}, fmt.Errorf("room: invalid deployment piece %q", string(ch))
		}
	}
	if !d.valid() {
		return Deployment{}, fmt.Errorf("room: deployment %q is not one king, one knight, and two pawns", order)
	}
	return d, nil
}

// ofenChar returns the OFEN letter for a deployable piece type and color.
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

// assembleDeployedOFEN builds the starting OFEN for a game from both players'
// blind deployments. White fills rank 1 left-to-right (files a..d); black fills
// rank 4 mirrored (the player's leftmost piece lands on file d), so equal
// orderings reproduce the symmetric standard position. All castle rights are
// granted (no piece has moved) and white is to move. The assembled position is
// validated through the octad library before being returned.
func assembleDeployedOFEN(white, black Deployment) (string, error) {
	if !white.valid() {
		return "", fmt.Errorf("room: invalid white deployment %v", white)
	}
	if !black.valid() {
		return "", fmt.Errorf("room: invalid black deployment %v", black)
	}

	var rank1, rank4 strings.Builder
	for i := 0; i < 4; i++ {
		rank1.WriteString(ofenChar(white[i], octad.White))
		// mirror black so each side deploys from its own perspective
		rank4.WriteString(ofenChar(black[3-i], octad.Black))
	}

	ofen := rank4.String() + "/4/4/" + rank1.String() + " w NCFncf - 0 1"

	// defensively confirm the assembled position parses as legal octad
	if _, err := octad.OFEN(ofen); err != nil {
		return "", fmt.Errorf("room: assembled deploy ofen %q invalid: %w", ofen, err)
	}
	return ofen, nil
}
