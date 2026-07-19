package clock

import (
	"strconv"
	"time"

	"github.com/dechristopher/octad/v2"
)

var flagged = CTime{t: 0}

// CTime is a wrapper for time.Duration that adds centi-second output
type CTime struct {
	t time.Duration
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

// UnmarshalJSON parses the seconds float emitted by MarshalJSON back into a
// CTime, so structures embedding TimeControl (e.g. the variant definition
// inside a persisted room snapshot) round-trip through JSON.
func (t *CTime) UnmarshalJSON(b []byte) error {
	secs, err := strconv.ParseFloat(string(b), 64)
	if err != nil {
		return err
	}
	t.t = time.Duration(secs * float64(time.Second))
	return nil
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
	// PreStart bounds the first-move grace period. The clock normally charges
	// no time before white's first move; with PreStart set, Start arms a
	// countdown that ends the grace when it expires — white goes on the clock
	// and can flag without ever moving. Zero keeps the unbounded grace.
	PreStart CTime `json:"ps"`
	// TODO extra time after x time passes?
	// TODO Bronstein delay?
}

// Centisecond represents one centi-second
const Centisecond = time.Second / 100

// Millisecond represents one millisecond
const Millisecond = time.Second / 1000

// FlipAck acknowledges a clock flip back to the room routine, reporting what
// the flip charged: the mover's think time for the ply just played (as charged
// — net of lag compensation and capped at their budget; zero on the uncharged
// first move) and their remaining budget after the flip (post-increment; zero
// when the flip flagged them).
type FlipAck struct {
	Think     CTime
	Remaining CTime
}

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
	// PreStart is the time remaining in the running pre-start countdown
	// (TimeControl.PreStart), zero once the game has commenced — via a first
	// move or the countdown expiring — or when no countdown is configured.
	PreStart CTime `json:"ps"`
}
