package player

// Player struct for keeping track of info, status, state, and score
type Player struct {
	ID          string
	IsBot       bool
	IsSpectator bool
	scorePoints int
	scoreHalf   int
	// sendLatency bool // TODO send server latency stats if enabled
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
