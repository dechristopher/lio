package player

import (
	"github.com/dechristopher/octad"

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

// Agreed returns true if both players have agreed
func (a Agreement) Agreed() bool {
	return util.BothColors(func(c octad.Color) bool {
		return a.agreed(c)
	})
}
