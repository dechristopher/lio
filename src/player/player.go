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

// Player struct for keeping track of info, status, state, and score.
// Spectators are never Players: they hold no seat, are tagged at the socket
// layer (channel.SocketContext.IsSpectator), and are flagged to the view via
// RoomTemplatePayload.IsSpectator.
type Player struct {
	ID          string
	IsBot       bool
	scorePoints int
	scoreHalf   int
	results     []GameResult
	// sendLatency bool // TODO send server latency stats if enabled
}

// Results returns the player's per-game match history in game order
func (p *Player) Results() []GameResult {
	return p.results
}

// resetScore clears the player's accumulated score and per-game history, for
// when a decided race-to match restarts as a fresh match in the same room.
func (p *Player) resetScore() {
	p.scorePoints = 0
	p.scoreHalf = 0
	p.results = nil
}

// Snapshot is the serializable form of a Player for room persistence: seat
// identity plus the accumulated match score and per-game history, which are
// otherwise unexported and would not survive a JSON round-trip.
type Snapshot struct {
	ID          string       `json:"id"`
	IsBot       bool         `json:"bot,omitempty"`
	ScorePoints int          `json:"sp,omitempty"`
	ScoreHalf   int          `json:"sh,omitempty"`
	Results     []GameResult `json:"res,omitempty"`
}

// Snapshot captures the player's persistable state.
func (p *Player) Snapshot() Snapshot {
	return Snapshot{
		ID:          p.ID,
		IsBot:       p.IsBot,
		ScorePoints: p.scorePoints,
		ScoreHalf:   p.scoreHalf,
		Results:     p.results,
	}
}

// RestorePlayer rebuilds a Player from a persisted snapshot.
func RestorePlayer(s Snapshot) *Player {
	return &Player{
		ID:          s.ID,
		IsBot:       s.IsBot,
		scorePoints: s.ScorePoints,
		scoreHalf:   s.ScoreHalf,
		results:     s.Results,
	}
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
