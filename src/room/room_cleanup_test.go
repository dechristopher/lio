package room

import (
	"testing"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/variant"
)

// TestHasOpenSeat covers the gate that decides whether a vacated waiting room is
// a junk open challenge to tear down instantly or a committed game to leave on
// the grace timer. It must be true only for an un-joined human challenge, and
// false the moment an opponent joins or for a bot room — that exclusion is what
// keeps the instant cleanup from firing during the challenge->board handoff.
func TestHasOpenSeat(t *testing.T) {
	t.Run("open challenge has an open seat", func(t *testing.T) {
		// white seat filled by the creator, black seat empty and joinable
		r := newTestInstance(t, "creator", "")
		if !r.hasOpenSeat() {
			t.Fatal("hasOpenSeat = false, want true for an un-joined challenge")
		}
	})

	t.Run("joined room has no open seat", func(t *testing.T) {
		r := newTestInstance(t, "creator", "opponent")
		if r.hasOpenSeat() {
			t.Fatal("hasOpenSeat = true, want false once both seats are filled")
		}
	})

	t.Run("bot room has no open seat", func(t *testing.T) {
		r := newBotTestInstance(t, "creator", octad.Black)
		if r.hasOpenSeat() {
			t.Fatal("hasOpenSeat = true, want false for a bot room")
		}
	})
}

// newBotTestInstance builds a minimal room with a human and a bot, without
// starting the room routine, mirroring newTestInstance.
func newBotTestInstance(t *testing.T, human string, botColor octad.Color) *Instance {
	t.Helper()

	cfg := game.OctadGameConfig{Variant: variant.HalfOneBlitz}
	g, err := game.NewOctadGame(cfg)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}

	players := player.Players{
		botColor:         &player.Player{IsBot: true},
		botColor.Other(): &player.Player{ID: human},
	}

	return &Instance{
		ID:           "testbotroom",
		creator:      human,
		stateMachine: newStateMachine(),
		params:       Params{Players: players, GameConfig: cfg},
		game:         g,
		players:      players,
		rematch:      player.Agreement{},
		done:         make(chan struct{}),
		joinToken:    "tok",
	}
}
