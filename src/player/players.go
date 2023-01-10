package player

import (
	"errors"
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/octad"
	"sync"
)

type Players struct {
	black *Player
	white *Player
	mut   *sync.RWMutex
}

// NewPlayers returns an initialized Players struct
func NewPlayers() *Players {
	return &Players{
		black: &Player{},
		white: &Player{},
		mut:   &sync.RWMutex{},
	}
}

func (p *Players) GetPlayers() (blackPlayer, whitePlayer *Player) {
	p.mut.RLock()
	defer p.mut.RUnlock()

	return p.black, p.white
}

// FlipColor flips which color the players are playing
func (p *Players) FlipColor() {
	p.mut.Lock()
	defer p.mut.Unlock()

	p.white, p.black = p.black, p.white
}

// MissingBlack returns true if the black player is missing
func (p *Players) MissingBlack() bool {
	p.mut.RLock()
	defer p.mut.RUnlock()

	if p.black.ID == "" && !p.black.IsBot {
		return true
	}
	return false
}

// MissingWhite returns true if the white player is missing
func (p *Players) MissingWhite() bool {
	p.mut.RLock()
	defer p.mut.RUnlock()

	if p.white.ID == "" && !p.white.IsBot {
		return true
	}
	return false
}

// AddPlayer returns true if the user ID is added as the specified player
func (p *Players) AddPlayer(uid string, playerColor octad.Color, vsBot bool) error {
	if playerColor == octad.White && p.MissingWhite() {
		p.mut.Lock()
		p.white.ID = uid
		if vsBot {
			p.black.IsBot = true
		}
		p.mut.Unlock()
		return nil
	}
	if playerColor == octad.Black && p.MissingBlack() {
		p.mut.Lock()
		p.black.ID = uid
		if vsBot {
			p.white.IsBot = true
		}
		p.mut.Unlock()
		return nil
	}
	// TODO add more detail
	return errors.New("failed to add player")
}

// AddMissingPlayer returns true if the user ID is added as a player along with the added players color
func (p *Players) AddMissingPlayer(uid string) (playerAdded bool, playerColor octad.Color) {
	if p.MissingBlack() {
		p.mut.Lock()
		p.black = &Player{
			ID: uid,
		}
		defer p.mut.Unlock()
		return true, octad.Black
	}
	if p.MissingWhite() {
		p.mut.Lock()
		p.white = &Player{
			ID: uid,
		}
		defer p.mut.Unlock()
		return true, octad.White
	}
	return false, octad.NoColor
}

// IsPlayer returns true if the given ID belongs to a player in this match
func (p *Players) IsPlayer(id string) bool {
	p.mut.RLock()
	defer p.mut.RUnlock()

	return p.black.ID == id || p.white.ID == id
}

// Lookup player by id and return the player instance and the color
func (p *Players) Lookup(id string) (*Player, octad.Color) {
	p.mut.RLock()
	defer p.mut.RUnlock()

	if p.black.ID == id {
		return p.black, octad.Black
	}
	if p.white.ID == id {
		return p.white, octad.White
	}
	return nil, octad.NoColor
}

// ScoreWin tracks a win for the given color
func (p *Players) ScoreWin(color octad.Color) {
	p.mut.Lock()
	defer p.mut.Unlock()

	if color == octad.Black {
		p.black.scorePoints++
	} else {
		p.white.scorePoints++
	}
}

// ScoreDraw tracks a draw (1/2 point) for both players
func (p *Players) ScoreDraw() {
	p.mut.Lock()
	defer p.mut.Unlock()

	p.black.scoreHalf++
	p.white.scoreHalf++
}

// ScoreMap returns a compatible ScorePayload map of the current player scores
// keyed by current player color
func (p *Players) ScoreMap() wsv1.ScorePayload {
	p.mut.RLock()
	defer p.mut.RUnlock()

	return wsv1.ScorePayload{
		White: float32(p.white.Score()),
		Black: float32(p.black.Score()),
	}
}

// GetBotColor returns the current color the bot is playing
func (p *Players) GetBotColor() octad.Color {
	p.mut.RLock()
	defer p.mut.RUnlock()

	if p.white.IsBot {
		return octad.White
	}
	if p.black.IsBot {
		return octad.Black
	}
	return octad.NoColor
}

// HasBot returns true if either player is configured to be a bot
func (p *Players) HasBot() bool {
	p.mut.RLock()
	defer p.mut.RUnlock()

	return p.black.IsBot || p.white.IsBot
}

// HasTwoBots returns true if both player are configured to be a bot
func (p *Players) HasTwoBots() bool {
	p.mut.RLock()
	defer p.mut.RUnlock()

	return p.black.IsBot && p.white.IsBot
}
