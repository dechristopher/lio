package room

import (
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/valyala/fastjson"

	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/www/ws/proto"
)

// These tests cover the follow-up findings in arch/DEPLOY_REMATCH_RACES.md:
// the game-identity plumbing that lets a client recover from a missed
// game-start broadcast, the SendMove guards that keep a WS read loop from
// parking on a channel nobody reads, the stale DeployStateMessage guard, and
// the deploy-channel drain between phases.

// gameIDFromState extracts the game id (d.i) from a board-state wire message,
// the same way the client reads it.
func gameIDFromState(raw []byte) string {
	return fastjson.GetString(raw, "d", "i")
}

// TestCurrentGameStateMessageCarriesGameID verifies every board snapshot is
// tagged with the game it belongs to, and that a rematch-style game swap
// changes the id — the signal a client uses to recognize a new game whose
// single-shot start broadcast it missed.
func TestCurrentGameStateMessageCarriesGameID(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	driveToOngoing(t, r)

	first := gameIDFromState(r.CurrentGameStateMessage(false, false))
	if first == "" {
		t.Fatal("board state carries no game id")
	}
	if first != r.game.ID {
		t.Fatalf("board state game id = %q, want %q", first, r.game.ID)
	}

	// swap in a fresh game the way the rematch reset does
	r.stateMu.Lock()
	ng, err := game.NewOctadGame(r.params.GameConfig)
	if err == nil {
		r.game = ng
	}
	r.stateMu.Unlock()
	if err != nil {
		t.Fatalf("new game: %v", err)
	}

	second := gameIDFromState(r.CurrentGameStateMessage(false, false))
	if second == first {
		t.Fatal("game id unchanged across a game swap; clients cannot detect the new game")
	}
}

// sendMoveReturns runs SendMove and reports whether it returned within the
// timeout — the property that keeps a WS read loop from parking forever on a
// channel no handler is reading.
func sendMoveReturns(r *Instance, mv *message.RoomMove, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		r.SendMove(mv)
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// TestSendMoveDroppedOutsideActiveStates confirms a move landing in a state
// with no moveChannel reader (deploy phase, game-over/rematch window) is
// dropped outright instead of parking the caller until the room closes.
func TestSendMoveDroppedOutsideActiveStates(t *testing.T) {
	t.Run("deploy phase", func(t *testing.T) {
		r := newTestInstance(t, "w", "b")
		r.moveChannel = make(chan *message.RoomMove)
		driveToDeploy(t, r)

		if !sendMoveReturns(r, &message.RoomMove{Move: proto.MovePayload{UOI: "c1c2"}}, time.Second) {
			t.Fatal("SendMove blocked during the deploy phase (no reader)")
		}
	})

	t.Run("game over", func(t *testing.T) {
		r := newTestInstance(t, "w", "b")
		r.moveChannel = make(chan *message.RoomMove)
		driveToOngoing(t, r)

		r.stateMu.Lock()
		r.game.Resign(octad.Black)
		r.stateMu.Unlock()
		if err := r.event(EventWhiteWinsResignation); err != nil {
			t.Fatalf("event: %v", err)
		}

		if !sendMoveReturns(r, &message.RoomMove{Move: proto.MovePayload{UOI: "c1c2"}}, time.Second) {
			t.Fatal("SendMove blocked during the game-over window (no reader)")
		}
	})
}

// TestSendMoveDroppedOnceDecided covers the game-end transition sliver: the
// outcome is decided but the FSM still reads StateGameOngoing (the same window
// as RequestRematch's race #3). A move accepted there would park the WS read
// loop — the moveChannel has no reader again until the next game — freezing
// that client's subsequent rematch click.
func TestSendMoveDroppedOnceDecided(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.moveChannel = make(chan *message.RoomMove)
	driveToOngoing(t, r)

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.stateMu.Unlock()
	if r.State() != StateGameOngoing {
		t.Fatalf("precondition: expected StateGameOngoing, got %s", r.State())
	}

	if !sendMoveReturns(r, &message.RoomMove{Move: proto.MovePayload{UOI: "c1c2"}}, time.Second) {
		t.Fatal("SendMove blocked in the decided-but-not-transitioned sliver")
	}
}

// TestSendMoveStampsGameID verifies a client move (which arrives with no game
// id) is stamped with the game it was sent for, so a send that parks across a
// rematch boundary is rejected by makeMove's staleness guard instead of being
// applied to the next game.
func TestSendMoveStampsGameID(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.moveChannel = make(chan *message.RoomMove)
	driveToOngoing(t, r)

	received := make(chan *message.RoomMove, 1)
	go func() {
		received <- <-r.moveChannel
	}()

	r.SendMove(&message.RoomMove{Move: proto.MovePayload{UOI: "c1c2"}})

	select {
	case mv := <-received:
		if mv.GameID != r.game.ID {
			t.Fatalf("move stamped with game id %q, want %q", mv.GameID, r.game.ID)
		}
	case <-time.After(time.Second):
		t.Fatal("move never delivered to the reader")
	}
}

// TestDeployStateMessageNilAfterPhase confirms the deploy-state snapshot is
// refused once deployAndStart has cleared the deadline: a message built from
// the cleared fields (zero seconds, no locks) delivered after the reveal would
// push a client back into deploy mode over a live game.
func TestDeployStateMessageNilAfterPhase(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	driveToDeploy(t, r)

	// phase live: a deploy-state message is produced
	r.stateMu.Lock()
	r.deployDeadline = time.Now().Add(30 * time.Second)
	r.stateMu.Unlock()
	if msg := r.DeployStateMessage("w"); msg == nil {
		t.Fatal("DeployStateMessage = nil while the phase is live")
	}

	// phase ended (deployAndStart cleared the deadline): refuse the snapshot
	r.stateMu.Lock()
	r.deployDeadline = time.Time{}
	r.stateMu.Unlock()
	if msg := r.DeployStateMessage("w"); msg != nil {
		t.Fatalf("DeployStateMessage produced a stale phase snapshot: %s", msg)
	}
}

// TestDrainDeployChannel confirms stragglers buffered after a deploy phase
// stopped reading are swept before the next phase begins, so they cannot be
// consumed as that phase's submissions (post-flip color, last game's
// arrangement).
func TestDrainDeployChannel(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.deployChannel = make(chan *message.RoomDeploy, deployChannelBuffer)

	r.deployChannel <- &message.RoomDeploy{Player: "w", Order: "nkpp"}
	r.deployChannel <- &message.RoomDeploy{Player: "b", Order: "knpp"}

	r.drainDeployChannel()

	select {
	case sub := <-r.deployChannel:
		t.Fatalf("deployChannel not drained: %+v", sub)
	default:
	}

	// draining an already-empty channel must be a no-op (and must not block)
	r.drainDeployChannel()
}
