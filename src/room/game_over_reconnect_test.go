package room

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/www/ws/proto"
)

// gameOverMsg unmarshals a GameOverStateMessage wire payload for assertions.
type gameOverMsg struct {
	Tag  string                `json:"t"`
	Data proto.GameOverPayload `json:"d"`
}

func parseGameOver(t *testing.T, b []byte) gameOverMsg {
	t.Helper()
	var msg gameOverMsg
	if err := json.Unmarshal(b, &msg); err != nil {
		t.Fatalf("unmarshal game-over message: %v", err)
	}
	if msg.Tag != string(proto.GameOverTag) {
		t.Fatalf("expected game-over tag %q, got %q", proto.GameOverTag, msg.Tag)
	}
	return msg
}

// TestGameOverStateMessageOngoing asserts a still-live game yields no game-over
// message, so the reconnect handler falls back to the normal board state.
func TestGameOverStateMessageOngoing(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	if msg := r.GameOverStateMessage(); msg != nil {
		t.Fatalf("expected nil for an ongoing game, got %s", msg)
	}
}

// TestGameOverStateMessageHuman asserts a finished human-vs-human game reports
// the decided outcome and the remaining manual rematch window (rw), so a
// reconnecting player re-enters the result overlay with an accurate countdown.
func TestGameOverStateMessageHuman(t *testing.T) {
	r := newTestInstance(t, "w", "b")

	// white wins by black's resignation
	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.stateMu.Unlock()

	// mid-window: 25s left on the human rematch window
	r.setRematchDeadline(time.Now().Add(25 * time.Second))

	msg := parseGameOver(t, r.GameOverStateMessage())

	if msg.Data.Winner != "w" {
		t.Fatalf("expected winner w, got %q", msg.Data.Winner)
	}
	if msg.Data.Reason != "resignation" {
		t.Fatalf("expected reason resignation, got %q", msg.Data.Reason)
	}
	if msg.Data.RematchWindow < 20 || msg.Data.RematchWindow > 25 {
		t.Fatalf("expected remaining window ~25s, got %d", msg.Data.RematchWindow)
	}
	if msg.Data.RoomOver {
		t.Fatal("a rematch-window game-over must not report room over")
	}
}

// TestGameOverStateMessageBot asserts a finished bot game carries no rematch
// countdown: bot games are neither auto-rematched nor time-boxed, so the room
// stays open for review + manual rematch (the client shows no countdown). Even a
// stray rematch deadline is ignored for a bot game.
func TestGameOverStateMessageBot(t *testing.T) {
	// bot plays White, human plays Black
	r := newBotTestInstance(t, "human", octad.White)

	r.stateMu.Lock()
	r.game.Resign(octad.Black) // bot (white) wins
	r.stateMu.Unlock()

	// a stray deadline must not produce a countdown for a bot game
	r.setRematchDeadline(time.Now().Add(5 * time.Second))

	msg := parseGameOver(t, r.GameOverStateMessage())

	if msg.Data.RematchWindow != 0 {
		t.Fatalf("bot game must not carry a rematch countdown, got %d", msg.Data.RematchWindow)
	}
	if msg.Data.RoomOver {
		t.Fatal("a finished bot game must not report room over")
	}
}

// TestGameOverStateMessageLapsedWindow asserts an unset/elapsed deadline yields a
// zero countdown rather than a negative one — the room is about to close or start
// the next game.
func TestGameOverStateMessageLapsedWindow(t *testing.T) {
	r := newTestInstance(t, "w", "b")

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.stateMu.Unlock()

	// deadline already in the past
	r.setRematchDeadline(time.Now().Add(-time.Second))

	msg := parseGameOver(t, r.GameOverStateMessage())
	if msg.Data.RematchWindow != 0 {
		t.Fatalf("expected zero countdown for a lapsed window, got rw=%d", msg.Data.RematchWindow)
	}
}
