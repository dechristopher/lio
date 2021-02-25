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

const Centi = time.Second / 100

type Command int

const (
	Flip Command = iota
)

type Victor int

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
