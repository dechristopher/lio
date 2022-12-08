package clock

import (
	"strconv"
	"time"

	"github.com/dechristopher/octad"
)

var flagged = CTime{t: 0}

// CTime is a wrapper for time.Duration that adds centi-second output
type CTime struct {
	t time.Duration
}

// Seconds returns the time in seconds
func (t CTime) Seconds() int64 {
	return int64(t.t)
}

// Centi returns the time in centi-seconds
func (t CTime) Centi() int64 {
	return int64(t.t / Centisecond)
}

// Milli returns the time in milliseconds
func (t CTime) Milli() int64 {
	return int64(t.t / Millisecond)
}

// Add will return the sum of the two times
func (t CTime) Add(time CTime) CTime {
	return ToCTime(t.t + time.t)
}

// Diff will return the difference between the two times
func (t CTime) Diff(time CTime) CTime {
	return ToCTime(t.t - time.t)
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

// Centisecond represents one centi-second
const Centisecond = time.Second / 100

// Millisecond represents one millisecond
const Millisecond = time.Second / 1000

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
	WhiteTime CTime       `json:"w"`
	BlackTime CTime       `json:"b"`
	Turn      octad.Color `json:"t"`
	IsPaused  bool        `json:"p"`
	Victor    Victor      `json:"v"`
}
