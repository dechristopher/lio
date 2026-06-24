package room

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/variant"
)

// newTestInstance builds a minimal room Instance without starting the room
// routine, so the synchronized accessors can be exercised directly under the
// race detector.
func newTestInstance(t *testing.T, white, black string) *Instance {
	t.Helper()

	cfg := game.OctadGameConfig{Variant: variant.HalfOneBlitz}
	g, err := game.NewOctadGame(cfg)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}

	players := player.Players{
		octad.White: &player.Player{ID: white},
		octad.Black: &player.Player{ID: black},
	}

	return &Instance{
		ID:           "testroom",
		creator:      white,
		stateMachine: newStateMachine(),
		params:       Params{Players: players, GameConfig: cfg},
		game:         g,
		players:      players,
		rematch:      player.Agreement{},
		done:         make(chan struct{}),
		joinToken:    "tok",
	}
}

// TestRoomConcurrentReadersAndWriter exercises the stateMu discipline: many
// reader goroutines (the public, self-locking accessors used by HTTP/WS
// handlers) run concurrently with a writer that both mutates the game and swaps
// the game pointer (a rematch). Under -race this fails if any read or write
// path touches game/players without holding stateMu (#9, and the octad lazy
// move-cache race).
func TestRoomConcurrentReadersAndWriter(t *testing.T) {
	r := newTestInstance(t, "w", "b")

	// drive the FSM to GameOngoing so CurrentGameStateMessage also exercises
	// the legal-move generation path (which lazily caches inside the game)
	if err := r.event(EventRoomInitialized); err != nil {
		t.Fatal(err)
	}
	if err := r.event(EventPlayersConnected); err != nil {
		t.Fatal(err)
	}
	if err := r.event(EventStartGame); err != nil {
		t.Fatal(err)
	}

	const iters = 2000
	var wg sync.WaitGroup

	// writer: mutate the game and periodically swap in a fresh game, all under
	// stateMu, mirroring makeMove's critical section and the rematch swap
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < iters; i++ {
			r.stateMu.Lock()
			moves := r.game.ValidMoves()
			if len(moves) > 0 && r.game.Outcome() == octad.NoOutcome {
				_ = r.game.Move(moves[0])
			} else {
				// game ended (or no moves): swap in a fresh game, simulating
				// the rematch pointer swap
				ng, err := game.NewOctadGame(r.params.GameConfig)
				if err == nil {
					r.game = ng
				}
			}
			r.stateMu.Unlock()
		}
	}()

	// readers: the cross-goroutine accessors
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iters; i++ {
				_ = r.CurrentGameStateMessage(true, false)
				_ = r.GameState()
				_ = r.GenTemplatePayload("w")
				_ = r.IsReady()
				_ = r.HasBot()
			}
		}()
	}

	wg.Wait()
}

// TestRoomConcurrentJoin verifies that when many goroutines race to join the
// same open seat with the same token, exactly one succeeds and the players map
// is never corrupted (#8). Under -race this also fails if Join writes the map
// without holding stateMu.
func TestRoomConcurrentJoin(t *testing.T) {
	// white seat filled, black seat open
	r := newTestInstance(t, "creator", "")

	const joiners = 64
	var (
		wg        sync.WaitGroup
		successes int32
	)

	for i := 0; i < joiners; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			if r.Join(fmt.Sprintf("joiner-%d", n), "tok") {
				atomic.AddInt32(&successes, 1)
			}
		}(i)
	}
	wg.Wait()

	if successes != 1 {
		t.Fatalf("expected exactly one successful join, got %d", successes)
	}

	// the open seat must now be filled by some joiner and consistent with the
	// game's recorded black player
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if r.players[octad.Black].ID == "" {
		t.Fatal("black seat still empty after a successful join")
	}
	if r.players[octad.Black].ID != r.game.Black {
		t.Fatalf("players map (%s) and game (%s) disagree on black",
			r.players[octad.Black].ID, r.game.Black)
	}
}
