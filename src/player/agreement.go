package player

import (
	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/util"
)

// Agreement between two players
type Agreement map[octad.Color]bool

// NewAgreement returns a new agreement instance
func NewAgreement() Agreement {
	return make(Agreement)
}

// Agree to whatever has been agreed upon
func (a Agreement) Agree(color octad.Color) {
	a[color] = true
}

// agreed returns true if the given player by color has agreed
func (a Agreement) agreed(color octad.Color) bool {
	return a[color]
}

// AgreedBy returns true if the given player by color has agreed. It exposes
// per-seat agreement so the room can report recorded rematch clicks back to
// clients (GameOverPayload.RematchWhite/RematchBlack), which is what lets a
// client detect and resend a click the server never received.
func (a Agreement) AgreedBy(color octad.Color) bool {
	return a.agreed(color)
}

// Agreed returns true if both players have agreed
func (a Agreement) Agreed() bool {
	return util.BothColors(func(c octad.Color) bool {
		return a.agreed(c)
	})
}
