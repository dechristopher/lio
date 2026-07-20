package room

import (
	"errors"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/www/ws/proto"
)

// TestDrainGateDropsMutations: once the drain gate is set, every inbound
// mutation entry point drops immediately — nothing may change a room after its
// final snapshot, and nothing may block a WS read loop on the way out.
func TestDrainGateDropsMutations(t *testing.T) {
	draining.Store(true)
	t.Cleanup(func() { draining.Store(false) })

	r := newTestInstance(t, "wp", "bp")
	driveToOngoing(t, r)

	// moves: returns instantly, delivers nothing
	if !sendMoveReturns(r, &message.RoomMove{Move: proto.MovePayload{UOI: "c1c2"}}, time.Second) {
		t.Fatal("SendMove blocked during drain")
	}
	select {
	case <-r.moveChannel:
		t.Fatal("gated move was delivered")
	default:
	}

	// in-game controls: dropped before the buffer
	r.sendControl(message.RoomControl{Type: message.Resign})
	select {
	case <-r.controlChannel:
		t.Fatal("gated control was delivered")
	default:
	}

	// rematch requests: dropped
	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.stateMu.Unlock()
	r.RequestRematch(channel.SocketContext{UID: "wp"})
	select {
	case <-r.controlChannel:
		t.Fatal("gated rematch was delivered")
	default:
	}

	// seats and lifecycle: refused
	if r.Join(player.Identity{UID: "newplayer"}, r.joinToken) {
		t.Fatal("join accepted during drain")
	}
	if r.Cancel() {
		t.Fatal("cancel accepted during drain")
	}

	// no new rooms
	if _, err := Create(Params{}); !errors.As(err, &ErrDraining{}) {
		t.Fatalf("Create during drain returned %v, want ErrDraining", err)
	}
}

// TestDrainFreezesAndFlushes: Drain stops every registered room's clock and
// synchronously flushes its final snapshot, and a move that raced past the
// gate cannot wedge the routine on the frozen clock (flipClock skips a paused
// clock instead of blocking on a goroutine that no longer exists).
func TestDrainFreezesAndFlushes(t *testing.T) {
	ms := newMemStore()
	installPersister(t, ms)
	t.Cleanup(func() { draining.Store(false) })

	r := newTestInstance(t, "wp", "bp")
	r.ID = "drainroom"
	driveToOngoing(t, r)
	r.game.Clock.Start()
	playTestMoves(t, r, 2)

	rooms.Store(r.ID, r)
	t.Cleanup(func() { rooms.Delete(r.ID) })

	Drain()

	if !Draining() {
		t.Fatal("drain did not set the gate")
	}
	if !r.game.Clock.State(true).IsPaused {
		t.Fatal("drain did not freeze the clock")
	}
	if _, ok := ms.get(r.ID); !ok {
		t.Fatal("drain did not flush the room's snapshot")
	}

	// the in-flight-move wedge regression: a makeMove against the frozen clock
	// must complete (skipping the flip), not block forever holding stateMu
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.stateMu.Lock()
		moves := r.game.ValidMoves()
		r.stateMu.Unlock()
		if len(moves) > 0 {
			r.makeMove(&message.RoomMove{
				Move: proto.MovePayload{UOI: moves[0].String()},
				Ctx:  channel.SocketContext{Channel: r.ID, UID: "wp", MT: 1},
			})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("makeMove wedged on the drain-frozen clock")
	}
}
