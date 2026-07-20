package room

import (
	"strings"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/looplab/fsm"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/variant"
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

// TestDeployPreservesJoinerUID is a regression test for the archive "BOT" bug:
// the joiner's seat uid must survive the deploy-phase game rebuild. A room is
// created with the creator seated and the other seat open, so GameConfig starts
// without the joiner's uid; Join must stamp the uid onto both r.game AND
// r.params.GameConfig, because deployAndStart rebuilds r.game from GameConfig at
// the end of the deploy phase. The old bug synced only r.game, so the rebuild
// wiped the joiner's uid — the first game then archived an empty seat uid, which
// the archive view rendered as "BOT".
func TestDeployPreservesJoinerUID(t *testing.T) {
	// mirror production Create: creator on white, black seat still open
	// (ID == "", so GameConfig.Black is empty until a joiner claims it)
	cfg := game.OctadGameConfig{Variant: variant.HalfOneBlitzDeploy, White: "creator"}
	g, err := game.NewOctadGame(cfg)
	if err != nil {
		t.Fatalf("new game: %v", err)
	}
	players := player.Players{
		octad.White: &player.Player{ID: "creator"},
		octad.Black: &player.Player{ID: ""}, // open seat (player.ToJoin)
	}
	r := &Instance{
		ID:           "testroom",
		creator:      "creator",
		stateMachine: newStateMachine(),
		params:       Params{Players: players, GameConfig: cfg},
		game:         g,
		players:      players,
		rematch:      player.Agreement{},
		draw:         player.Agreement{},
		done:         make(chan struct{}),
		joinToken:    "tok",
	}

	// the joiner claims the open black seat
	uid := int64(42)
	if !r.Join(player.Identity{UID: "joiner", UserID: &uid}, "tok") {
		t.Fatal("join failed")
	}
	if r.game.Black != "joiner" {
		t.Fatalf("after join, game.Black = %q, want %q", r.game.Black, "joiner")
	}

	driveToDeploy(t, r)

	done := make(chan struct{})
	go func() {
		r.handleDeploy()
		close(done)
	}()
	submitDeploy(r, "creator", "nkpp")
	submitDeploy(r, "joiner", "nkpp")
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("deploy did not complete in time")
	}

	// the rebuilt game must still carry both seats' uids
	if r.game.White != "creator" {
		t.Errorf("after deploy rebuild, game.White = %q, want %q", r.game.White, "creator")
	}
	if r.game.Black != "joiner" {
		t.Errorf("after deploy rebuild, game.Black = %q, want %q (deploy wiped the joiner uid)", r.game.Black, "joiner")
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
