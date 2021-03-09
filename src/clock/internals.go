package clock

import "time"

// TimeControl stores the start time per player
// and information about the game's increment or delay
type TimeControl struct {
	Time      time.Duration // time in seconds
	Increment time.Duration // seconds gained after each move
	Delay     time.Duration // seconds before time starts to decrement
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

type ClockTime struct {
	t time.Duration
}

func (t ClockTime) Centi() int64 {
	return int64(t.t / Centi)
}

// State represents the current state of the Clock
type State struct {
	BlackTime ClockTime `json:"b"`
	WhiteTime ClockTime `json:"w"`
	IsBlack   bool      `json:"t"`
	IsPaused  bool      `json:"p"`
	Victor    Victor    `json:"v"`
}
