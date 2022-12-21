package clock

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"time"

	"github.com/dechristopher/octad"
)

// Command constant for clock operations
type Command int

// Flip clock command
const Flip Command = iota

// Victor of the game
type Victor int

// Possible victor states according to the clock
const (
	NoVictor Victor = iota
	Black
	White
)

// State represents the current state of the Clock
type State struct {
	WhiteTime time.Duration `json:"w"`
	BlackTime time.Duration `json:"b"`
	Turn      octad.Color   `json:"t"`
	IsPaused  bool          `json:"p"`
	Victor    Victor        `json:"v"`
}

// ConvertVariantTimeControl modifies a variant time control to be in milliseconds instead of nanoseconds. This is
// necessary because the client can only work in milliseconds
func ConvertVariantTimeControl(variant *wsv1.Variant) *wsv1.Variant {
	return &wsv1.Variant{
		Name:     variant.Name,
		HtmlName: variant.HtmlName,
		Group:    variant.Group,
		Control: &wsv1.TimeControl{
			InitialTime: time.Duration(variant.Control.InitialTime).Milliseconds(),
			Increment:   time.Duration(variant.Control.Increment).Milliseconds(),
			Delay:       time.Duration(variant.Control.Delay).Milliseconds(),
		},
	}
}
