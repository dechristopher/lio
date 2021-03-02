package clock

import "time"

// TimeControl stores the start time per player
// and information about the game's increment or delay
type TimeControl struct {
	Time      time.Duration
	Increment time.Duration
	Delay     time.Duration
	// TODO extra time after x time passes?
	// TODO Bronstein delay?
}

// Centi represents one centi-second
const Centi = time.Second / 100

// Command constant for clock operations
type Command int

// Clock commands, not many so far
const (
	Flip Command = iota
)

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
	BlackTime time.Duration `json:"b"`
	WhiteTime time.Duration `json:"w"`
	IsBlack   bool          `json:"t"`
	IsPaused  bool          `json:"p"`
	Victor    Victor        `json:"v"`
}
