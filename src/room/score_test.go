package room

import (
	"encoding/json"
	"testing"

	"github.com/dechristopher/octad/v2"
)

// moveMsg unmarshals just the seating + score fields of a MovePayload wire
// message (the full payload's clock.CTime fields don't round-trip through
// encoding/json, and they're irrelevant here).
type moveMsg struct {
	Data struct {
		White string             `json:"w"`
		Black string             `json:"b"`
		Score map[string]float64 `json:"sc"`
	} `json:"d"`
}

// TestScoreAttributionOnLoss guards against crediting the wrong side at game
// over: when White wins, the point must land on White in both the board-state
// message (which drives the room clocks) and the game-over message (which drives
// the result overlay), never on the losing Black player.
func TestScoreAttributionOnLoss(t *testing.T) {
	// bot plays White, human "human" plays Black
	r := newBotTestInstance(t, "human", octad.White)

	// white wins by black resigning, then apply the match-score update the room
	// routine performs at game over
	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.updateScoreLocked()
	r.stateMu.Unlock()

	// board-state message → room clocks
	var state moveMsg
	if err := json.Unmarshal(r.CurrentGameStateMessage(true, false), &state); err != nil {
		t.Fatalf("unmarshal board state: %v", err)
	}
	if state.Data.Black != "human" || state.Data.White == "human" {
		t.Fatalf("expected human seated on black: white=%q black=%q",
			state.Data.White, state.Data.Black)
	}
	if got := state.Data.Score["w"]; got != 1 {
		t.Errorf("board-state white score = %v, want 1 (white won)", got)
	}
	if got := state.Data.Score["b"]; got != 0 {
		t.Errorf("board-state black (human) score = %v, want 0 (human lost)", got)
	}

	// game-over message → result overlay
	over := parseGameOver(t, r.GameOverStateMessage())
	if over.Data.Winner != "w" {
		t.Fatalf("winner = %q, want w", over.Data.Winner)
	}
	if got := over.Data.Score["w"]; got != 1 {
		t.Errorf("game-over white score = %v, want 1", got)
	}
	if got := over.Data.Score["b"]; got != 0 {
		t.Errorf("game-over black (human) score = %v, want 0", got)
	}
}
