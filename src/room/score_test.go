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
		White   string             `json:"w"`
		Black   string             `json:"b"`
		Score   map[string]float64 `json:"sc"`
		History []struct {
			White       float64 `json:"w"`
			Black       float64 `json:"b"`
			Reason      string  `json:"r"`
			WhitePlayed string  `json:"wp"`
		} `json:"h"`
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
		t.Errorf("board-state white score = %v, want 1 (white wins)", got)
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

// TestMatchHistoryRekeysAcrossColorFlip guards the match-history seat
// convention: entries are keyed by the players' CURRENT seats (like the score),
// so after the between-games color flip a past game's points must follow the
// player to their new seat, and WhitePlayed must report the color the
// now-white player actually held in that game.
func TestMatchHistoryRekeysAcrossColorFlip(t *testing.T) {
	// bot plays White, human "human" plays Black
	r := newBotTestInstance(t, "human", octad.White)

	// white wins game 1 by black resigning
	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.updateScoreLocked()
	r.stateMu.Unlock()

	var state moveMsg
	if err := json.Unmarshal(r.CurrentGameStateMessage(true, false), &state); err != nil {
		t.Fatalf("unmarshal board state: %v", err)
	}
	if len(state.Data.History) != 1 {
		t.Fatalf("history length = %d, want 1", len(state.Data.History))
	}
	e := state.Data.History[0]
	if e.White != 1 || e.Black != 0 {
		t.Errorf("game 1 points = w:%v b:%v, want w:1 b:0", e.White, e.Black)
	}
	if e.Reason != "resignation" {
		t.Errorf("game 1 reason = %q, want resignation", e.Reason)
	}
	if e.WhitePlayed != "w" {
		t.Errorf("game 1 whitePlayed = %q, want w (no flip yet)", e.WhitePlayed)
	}

	// the rematch color swap: the human now holds the white seat
	r.stateMu.Lock()
	r.players.FlipColor()
	r.stateMu.Unlock()

	if err := json.Unmarshal(r.CurrentGameStateMessage(true, false), &state); err != nil {
		t.Fatalf("unmarshal board state after flip: %v", err)
	}
	if state.Data.White != "human" {
		t.Fatalf("expected human seated on white after flip: white=%q", state.Data.White)
	}
	if len(state.Data.History) != 1 {
		t.Fatalf("history length after flip = %d, want 1", len(state.Data.History))
	}
	e = state.Data.History[0]
	// the human lost game 1 and now sits white: their 0 follows them
	if e.White != 0 || e.Black != 1 {
		t.Errorf("game 1 points after flip = w:%v b:%v, want w:0 b:1", e.White, e.Black)
	}
	// the now-white human played black in game 1
	if e.WhitePlayed != "b" {
		t.Errorf("game 1 whitePlayed after flip = %q, want b", e.WhitePlayed)
	}
}
