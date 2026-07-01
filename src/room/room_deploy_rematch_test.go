package room

import (
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/looplab/fsm"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
)

// driveToOngoing advances the FSM from init into the live game state.
func driveToOngoing(t *testing.T, r *Instance) {
	t.Helper()
	for _, ev := range []fsm.EventDesc{EventRoomInitialized, EventPlayersConnected, EventStartGame} {
		if err := r.event(ev); err != nil {
			t.Fatalf("event %s: %v", ev.Name, err)
		}
	}
	if r.State() != StateGameOngoing {
		t.Fatalf("expected StateGameOngoing, got %s", r.State())
	}
}

// TestRequestRematchDecidedOutcomeWindow covers race #3: a rematch click that
// lands in the sliver between the game-over broadcast and the FSM transition
// into StateGameOver (State() still reads StateGameOngoing) must still be
// accepted, because the game already has a terminal outcome. A click while the
// game is genuinely undecided is still dropped.
func TestRequestRematchDecidedOutcomeWindow(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.controlChannel = make(chan message.RoomControl, 2)
	driveToOngoing(t, r)

	// undecided game, still ongoing: a click is not a rematch and must be dropped
	r.RequestRematch(channel.SocketContext{UID: "w"})
	select {
	case ctrl := <-r.controlChannel:
		t.Fatalf("rematch accepted before the game was decided: %+v", ctrl)
	default:
	}

	// decide the game while the room is still in StateGameOngoing — this models
	// the pre-transition window where the client has already seen the game-over
	// message and pressed rematch
	r.stateMu.Lock()
	r.game.Resign(octad.Black) // white wins by resignation
	r.stateMu.Unlock()
	if r.State() != StateGameOngoing {
		t.Fatalf("precondition: expected StateGameOngoing, got %s", r.State())
	}

	r.RequestRematch(channel.SocketContext{UID: "w"})
	select {
	case ctrl := <-r.controlChannel:
		if ctrl.Type != message.Rematch {
			t.Fatalf("expected Rematch control, got %v", ctrl.Type)
		}
	default:
		t.Fatal("rematch dropped despite a decided outcome in the pre-transition window")
	}
}

// TestRequestRematchRejectsNonPlayer confirms the seated-player guard still
// holds under the widened outcome window: a spectator's click is dropped even
// once the game is decided.
func TestRequestRematchRejectsNonPlayer(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.controlChannel = make(chan message.RoomControl, 2)
	driveToOngoing(t, r)

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.stateMu.Unlock()

	r.RequestRematch(channel.SocketContext{UID: "spectator"})
	select {
	case ctrl := <-r.controlChannel:
		t.Fatalf("rematch accepted from a non-player: %+v", ctrl)
	default:
	}
}

// TestSubmitDeployNeverBlocks covers race #2: SubmitDeploy is called from a
// client's serial WS read loop, so it must never block even when the deploy
// handler is not reading the channel (the deployAndStart window) and the buffer
// is full. It exercises more submissions than the buffer holds and asserts they
// all return promptly rather than wedging the caller.
func TestSubmitDeployNeverBlocks(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	// driveToDeploy leaves the room in StateDeploy with a buffered deployChannel
	// but does NOT start handleDeploy, so nothing is reading the channel.
	driveToDeploy(t, r)

	done := make(chan struct{})
	go func() {
		// well beyond deployChannelBuffer; the overflow must be dropped, not block
		for i := 0; i < deployChannelBuffer+5; i++ {
			submitDeploy(r, "w", "nkpp")
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SubmitDeploy blocked when the deploy handler was not reading")
	}
}

// TestSubmitDeployDropsOutsidePhase confirms a submission arriving when the room
// is no longer in the deploy phase is dropped by the state guard rather than
// enqueued onto a channel nobody drains.
func TestSubmitDeployDropsOutsidePhase(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.deployChannel = make(chan *message.RoomDeploy, deployChannelBuffer)
	driveToOngoing(t, r) // not StateDeploy

	submitDeploy(r, "w", "nkpp")
	select {
	case sub := <-r.deployChannel:
		t.Fatalf("deploy accepted outside the deploy phase: %+v", sub)
	default:
	}
}

// TestDrainControlChannel covers the race #3 hardening: stale controls buffered
// from a finished game-over are discarded so they cannot be misread as a rematch
// agreement in the next game.
func TestDrainControlChannel(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.controlChannel = make(chan message.RoomControl, 2)

	r.controlChannel <- message.RoomControl{Type: message.Rematch}
	r.controlChannel <- message.RoomControl{Type: message.Rematch}

	r.drainControlChannel()

	select {
	case ctrl := <-r.controlChannel:
		t.Fatalf("controlChannel not drained: %+v", ctrl)
	default:
	}

	// draining an already-empty channel must be a no-op (and must not block)
	r.drainControlChannel()
}
