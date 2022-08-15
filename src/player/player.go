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

// Score returns the player's match score
func (p *Player) Score() float64 {
	score := 0.0

	score += float64(p.scorePoints)
	score += 0.5 * float64(p.scoreHalf)

	return score
}
