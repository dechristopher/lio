package engine

import (
	"strings"
)

// Repetition awareness for the search. Search reconstructs the game from a
// bare OFEN, so octad's automatic threefold-repetition draw can never trigger
// against positions from the real game: the engine would happily shuffle a won
// endgame into a draw (and did — checking a lone king back and forth until the
// room recorded the automatic threefold). The caller therefore passes the
// game's position history into Search, and the minimax search scores any node
// that revisits one of those positions — or completes a genuine threefold
// within its own line — as the draw it is (or is about to become).

// repetitionKey normalizes an OFEN to the fields that define position identity
// for repetition purposes: piece placement, side to move, castling rights, and
// en passant square. The halfmove clock and fullmove number are dropped — they
// always differ between repetitions of the same position.
func repetitionKey(ofen string) string {
	fields := strings.Fields(ofen)
	if len(fields) < 4 {
		return ofen
	}
	return strings.Join(fields[:4], " ")
}

// RepetitionHistory converts a game's position history (OFENs oldest-first,
// including the current position — the shape of game.OctadGame.OFENHistory)
// into the occurrence-count map Search consumes. A nil or empty slice yields
// nil, which disables repetition scoring (the pre-history behavior).
func RepetitionHistory(ofens []string) map[string]int {
	if len(ofens) == 0 {
		return nil
	}
	hist := make(map[string]int, len(ofens))
	for _, o := range ofens {
		hist[repetitionKey(o)]++
	}
	return hist
}

// repTracker carries repetition state through one root move's search line:
// hist holds the real game's occurrence counts (shared read-only across the
// parallel root goroutines) and path counts the positions entered along the
// current line, maintained make/undo style. A nil *repTracker disables all
// repetition checks.
type repTracker struct {
	hist map[string]int
	path map[string]int
}

// newRepTracker returns a tracker over hist, or nil when there is no history
// to track.
func newRepTracker(hist map[string]int) *repTracker {
	if hist == nil {
		return nil
	}
	return &repTracker{hist: hist, path: make(map[string]int)}
}

// isDrawnRepetition reports whether the position keyed by key, just reached in
// the search, should score as a draw: it already occurred in the real game
// (revisiting proves no progress, and the side that wants the draw can keep
// steering back until the automatic threefold lands), or the search line
// itself contains a genuine threefold (two prior occurrences plus this one).
func (rt *repTracker) isDrawnRepetition(key string) bool {
	return rt.hist[key] > 0 || rt.path[key] >= 2
}

// enter/leave bracket a node's time on the current search line, mirroring the
// game's Move/UndoMove pair around the recursive call.
func (rt *repTracker) enter(key string) { rt.path[key]++ }
func (rt *repTracker) leave(key string) { rt.path[key]-- }
