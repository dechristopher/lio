package player

import (
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/octad"
)

// Players map for use anywhere two players compete
type Players map[octad.Color]*Player

// FlipColor flips which color the players are playing
func (p Players) FlipColor() {
	white := p[octad.White]
	p[octad.White] = p[octad.Black]
	p[octad.Black] = white
}

// HasTwoPlayers returns true if both players are configured, and
// the color of the missing player if only one player is missing
func (p Players) HasTwoPlayers() (hasTwo bool, missing octad.Color) {
	hasTwo = util.BothColors(func(color octad.Color) bool {
		return p[color] != nil
	})

	if p[octad.White] == nil {
		missing = octad.White
	} else if p[octad.Black] == nil {
		missing = octad.Black
	} else {
		missing = octad.NoColor
	}

	return
}

// IsPlayer returns true if the given ID belongs to a player in this match
func (p Players) IsPlayer(id string) bool {
	return p[octad.White].ID == id || p[octad.Black].ID == id
}

// Lookup player by id and return the player instance and the color
func (p Players) Lookup(id string) (*Player, octad.Color) {
	if p[octad.White].ID == id {
		return p[octad.White], octad.White
	}
	if p[octad.Black].ID == id {
		return p[octad.Black], octad.Black
	}
	return nil, octad.NoColor
}

// ScoreWin tracks a win for the given color
func (p Players) ScoreWin(color octad.Color) {
	p[color].scorePoints++
}

// ScoreDraw tracks a draw (1/2 point) for both players
func (p Players) ScoreDraw() {
	util.DoBothColors(func(c octad.Color) {
		p[c].scoreHalf++
	})
}

// GetBotColor returns the current color the bot is playing
func (p Players) GetBotColor() octad.Color {
	if p[octad.White].IsBot {
		return octad.White
	}
	if p[octad.Black].IsBot {
		return octad.Black
	}
	return octad.NoColor
}

// HasBot returns true if either player is configured to be a bot
func (p Players) HasBot() bool {
	return p.GetBotColor() != octad.NoColor
}
