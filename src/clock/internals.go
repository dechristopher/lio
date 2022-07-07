package clock

import (
	"strconv"
	"time"
)

// CTime is a wrapper for time.Duration that adds centi-second output
type CTime struct {
	t time.Duration
}

// Centi returns the time left in centi-seconds
func (t CTime) Centi() int64 {
	return int64(t.t / Centi)
}

// MarshalJSON marshals CTime as an integer instead of
// the default formatted string default in time.Duration
func (t CTime) MarshalJSON() ([]byte, error) {
	if t.t.Seconds() == float64(int(t.t.Seconds())) {
		return []byte(strconv.FormatFloat(t.t.Seconds(),
			'f', 1, 32)), nil
	}
	return []byte(strconv.FormatFloat(t.t.Seconds(),
		'f', -1, 32)), nil
}

// String returns the string representation of
// the internal time.Duration for pretty printing
func (t CTime) String() string {
	return t.t.String()
}

// ToCTime wraps a time.Duration in CTime
func ToCTime(duration time.Duration) CTime {
	return CTime{t: duration}
}

// TimeControl stores the start time per player
// and information about the game's increment or delay
type TimeControl struct {
	Time      CTime `json:"t"` // time in seconds
	Increment CTime `json:"i"` // seconds gained after each move
	Delay     CTime `json:"d"` // seconds before time starts to decrement
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
	Resign
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
	BlackTime CTime  `json:"b"`
	WhiteTime CTime  `json:"w"`
	IsBlack   bool   `json:"t"`
	IsPaused  bool   `json:"p"`
	Victor    Victor `json:"v"`
}
