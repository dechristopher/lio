package player

import "github.com/dechristopher/octad/v2"

// GameResult records one finished game of a room's match from this player's
// perspective: the points they earned (1, 0.5, or 0), the color they played
// that game, and the short method code describing how the game ended (the
// same codes as GameOverPayload.Reason: checkmate, time, resignation, ...).
// Like the score counters, results travel with the *Player through
// Players.FlipColor, so the history survives the color swaps between games.
type GameResult struct {
	Points float64
	Color  octad.Color
	Reason string
}

// Player struct for keeping track of info, status, state, and score
type Player struct {
	ID          string
	IsBot       bool
	IsSpectator bool
	scorePoints int
	scoreHalf   int
	results     []GameResult
	// sendLatency bool // TODO send server latency stats if enabled
}

// Results returns the player's per-game match history in game order
func (p *Player) Results() []GameResult {
	return p.results
}

// ToJoin is a sample Player used to configure a room in which
// the opponent joins via URL and is then configured
var ToJoin = Player{
	ID:    "",
	IsBot: false,
}

// Score returns the player's match score
func (p *Player) Score() float64 {
	score := 0.0

	score += float64(p.scorePoints)
	score += 0.5 * float64(p.scoreHalf)

	return score
}
