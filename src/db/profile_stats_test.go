package db

import "testing"

// TestSeatScore locks the per-game result derivation. This is the fix for a real
// bug: the archive's white_score/black_score columns hold the *cumulative match
// score* (room.go archives player.Score(), which accumulates across a room's
// games), so from game 2 of a match onward they are not the game's result. A
// 3-game match reaching 1.5 matched none of the =1 / =0.5 / =0 filters at all,
// so those games vanished from every profile tally whose total still counted
// them.
func TestSeatScore(t *testing.T) {
	cases := []struct {
		outcome string
		asWhite bool
		want    float32
	}{
		{"1-0", true, 1},  // White won, account was White
		{"1-0", false, 0}, // White won, account was Black
		{"0-1", false, 1}, // Black won, account was Black
		{"0-1", true, 0},  // Black won, account was White
		{"1/2-1/2", true, 0.5},
		{"1/2-1/2", false, 0.5},
		// an unrecognised outcome must not silently score as a win
		{"*", true, 0},
		{"", false, 0},
	}
	for _, c := range cases {
		if got := SeatScore(c.outcome, c.asWhite); got != c.want {
			t.Errorf("SeatScore(%q, asWhite=%v) = %v, want %v",
				c.outcome, c.asWhite, got, c.want)
		}
	}
}

// TestSeatScoreIsZeroSum checks the two seats of any decided game always total
// exactly 1 — the property the cumulative score columns violated.
func TestSeatScoreIsZeroSum(t *testing.T) {
	for _, outcome := range []string{"1-0", "0-1", "1/2-1/2"} {
		if sum := SeatScore(outcome, true) + SeatScore(outcome, false); sum != 1 {
			t.Errorf("outcome %q: seats total %v, want 1", outcome, sum)
		}
	}
}
