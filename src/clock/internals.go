package clock

import "time"

// TimeControl stores the start time per player
// and information about the game's increment or delay
type TimeControl struct {
	Time      time.Duration `json:"t"` // time in seconds
	Increment time.Duration `json:"i"` // seconds gained after each move
	Delay     time.Duration `json:"d"` // seconds before time starts to decrement
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

// CTime is a wrapper for time.Duration that adds centi-second output
type CTime struct {
	t time.Duration
}

// Centi returns the time left in centi-seconds
func (t CTime) Centi() int64 {
	return int64(t.t / Centi)
}

// State represents the current state of the Clock
type State struct {
	BlackTime CTime  `json:"b"`
	WhiteTime CTime  `json:"w"`
	IsBlack   bool   `json:"t"`
	IsPaused  bool   `json:"p"`
	Victor    Victor `json:"v"`
}
