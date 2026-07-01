package room

import (
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/looplab/fsm"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
)

// driveToDeploy advances the FSM from init into the deploy phase.
func driveToDeploy(t *testing.T, r *Instance) {
	t.Helper()
	r.params.Deploy = true
	// buffered to match production (Create): with the non-blocking SubmitDeploy
	// send, an unbuffered channel would let a submission race the handler's start
	// and be dropped by the default case. See deployChannelBuffer.
	r.deployChannel = make(chan *message.RoomDeploy, deployChannelBuffer)
	for _, ev := range []fsm.EventDesc{EventRoomInitialized, EventPlayersConnected, EventStartDeploy} {
		if err := r.event(ev); err != nil {
			t.Fatalf("event %s: %v", ev.Name, err)
		}
	}
	if r.State() != StateDeploy {
		t.Fatalf("expected StateDeploy, got %s", r.State())
	}
}

func submitDeploy(r *Instance, player, order string) {
	r.SubmitDeploy(&message.RoomDeploy{
		Player: player,
		Order:  order,
		Ctx:    channel.SocketContext{Channel: r.ID, RoomID: r.ID, UID: player},
	})
}

// standardStartOFEN returns the library's canonical starting OFEN.
func standardStartOFEN() string {
	g, err := octad.NewGame()
	if err != nil {
		panic(err)
	}
	return g.OFEN()
}

// TestDeployPhaseHumanGame drives a human-vs-human deploy: both players submit
// their arrangements and the game begins from the assembled position.
func TestDeployPhaseHumanGame(t *testing.T) {
	r := newTestInstance(t, "white", "black")
	driveToDeploy(t, r)

	done := make(chan struct{})
	go func() {
		r.handleDeploy()
		close(done)
	}()

	// white deploys king-first (knpp); black deploys standard (nkpp)
	submitDeploy(r, "white", "knpp")
	submitDeploy(r, "black", "nkpp")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deploy did not complete in time")
	}

	if r.State() != StateGameOngoing {
		t.Fatalf("expected StateGameOngoing after deploy, got %s", r.State())
	}
	want := "ppkn/4/4/KNPP w NCFncf - 0 1"
	if got := r.game.OFEN(); got != want {
		t.Fatalf("deployed OFEN = %q, want %q", got, want)
	}
}

// TestDeployPhaseTimeoutAutofill verifies that when nobody submits, the deploy
// timer fires and both sides are auto-filled with the standard ordering, which
// reproduces the classic starting position.
func TestDeployPhaseTimeoutAutofill(t *testing.T) {
	prev := deployTimeout
	deployTimeout = 30 * time.Millisecond
	defer func() { deployTimeout = prev }()

	r := newTestInstance(t, "white", "black")
	driveToDeploy(t, r)

	done := make(chan struct{})
	go func() {
		r.handleDeploy()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deploy did not auto-complete after timeout")
	}

	if r.State() != StateGameOngoing {
		t.Fatalf("expected StateGameOngoing after timeout, got %s", r.State())
	}
	if got := r.game.OFEN(); got != standardStartOFEN() {
		t.Fatalf("timeout-autofilled OFEN = %q, want standard start %q", got, standardStartOFEN())
	}
}

// TestDeployPhaseBotAutodeploy verifies a bot (Black) deploys itself so only the
// human's submission is needed to start the game.
func TestDeployPhaseBotAutodeploy(t *testing.T) {
	r := newBotTestInstance(t, "human", octad.Black)
	driveToDeploy(t, r)

	done := make(chan struct{})
	go func() {
		r.handleDeploy()
		close(done)
	}()

	// the human plays White; deploy king-first
	submitDeploy(r, "human", "knpp")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deploy did not complete with bot auto-deploy")
	}

	if r.State() != StateGameOngoing {
		t.Fatalf("expected StateGameOngoing, got %s", r.State())
	}
	// white's home rank (last board segment) must reflect the human's KNPP order
	board := strings.Split(r.game.OFEN(), " ")[0]
	ranks := strings.Split(board, "/")
	if ranks[3] != "KNPP" {
		t.Fatalf("white rank = %q, want KNPP (full ofen %q)", ranks[3], r.game.OFEN())
	}
	if len(r.game.ValidMoves()) == 0 {
		t.Fatal("deployed game should have legal moves")
	}
}
