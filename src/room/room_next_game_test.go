package room

import (
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/valyala/fastjson"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
)

// sendNextGame pushes a next-game control straight onto the room's control
// channel, bypassing RequestNextGame's caller guards (which have their own
// test below).
func sendNextGame(r *Instance, uid string) {
	r.controlChannel <- message.RoomControl{
		Type: message.NextGame,
		Ctx:  channel.SocketContext{UID: uid},
	}
}

// TestGameOverMessageCarriesGameID verifies every game-over payload names the
// game it describes, and that the id follows the game across a boundary. It is
// what lets a client drop a payload built just before a reset but delivered
// after the next game started — which would otherwise apply the finished
// game's score under the new seat colors and re-raise a stale result card.
func TestGameOverMessageCarriesGameID(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	driveToOngoing(t, r)

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	finished := r.game.ID
	live := r.gameOverMessageLocked(false, "")
	r.stateMu.Unlock()

	if got := fastjson.GetString(live, "d", "gi"); got != finished {
		t.Fatalf("live game-over game id = %q, want %q", got, finished)
	}

	if err := r.event(EventWhiteWinsResignation); err != nil {
		t.Fatalf("event: %v", err)
	}

	// the reconnect/resync payload describes the same game
	if got := fastjson.GetString(r.GameOverStateMessage(), "d", "gi"); got != finished {
		t.Fatalf("resync game-over game id = %q, want %q", got, finished)
	}

	// after the boundary the room's id has moved on, so a payload built for the
	// finished game no longer matches what a client is tracking
	r.stateMu.Lock()
	err := r.resetForNextGameLocked(false)
	next := r.game.ID
	r.stateMu.Unlock()
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if next == finished {
		t.Fatal("game id did not change across the game boundary")
	}
}

// TestGameOverMessageCarriesNextGameReadiness verifies a mid-match game-over
// payload reports the server's recorded per-seat interlude readiness, which is
// what lets a client's resync poll detect a "next game" click that never
// arrived (and resend it) or restore its ready state after a reload.
func TestGameOverMessageCarriesNextGameReadiness(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.params.RaceTo = 3
	driveToOngoing(t, r)

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.updateScoreLocked()
	r.stateMu.Unlock()
	if err := r.event(EventWhiteWinsResignation); err != nil {
		t.Fatalf("event: %v", err)
	}

	// nothing recorded yet: both seats read un-ready
	raw := r.GameOverStateMessage()
	if raw == nil {
		t.Fatal("GameOverStateMessage = nil for a finished game")
	}
	if fastjson.GetBool(raw, "d", "ngw") || fastjson.GetBool(raw, "d", "ngb") {
		t.Fatalf("readiness reported before any click: %s", raw)
	}

	r.stateMu.Lock()
	r.nextGame.Agree(octad.Black)
	r.stateMu.Unlock()

	raw = r.GameOverStateMessage()
	if fastjson.GetBool(raw, "d", "ngw") {
		t.Fatalf("white reported ready after only black asked: %s", raw)
	}
	if !fastjson.GetBool(raw, "d", "ngb") {
		t.Fatalf("black's recorded readiness missing from payload: %s", raw)
	}
}

// TestRequestNextGameGuards locks the caller guards: the request is accepted
// only from a seated player of an undecided race-to match whose game is
// decided — including in the sliver before the FSM reaches StateGameOver (the
// same window RequestRematch accepts, since the game-over broadcast that shows
// the button precedes the transition).
func TestRequestNextGameGuards(t *testing.T) {
	drained := func(t *testing.T, r *Instance) bool {
		t.Helper()
		select {
		case <-r.controlChannel:
			return true
		default:
			return false
		}
	}

	t.Run("undecided game", func(t *testing.T) {
		r := newTestInstance(t, "w", "b")
		r.params.RaceTo = 3
		r.controlChannel = make(chan message.RoomControl, 2)
		driveToOngoing(t, r)

		r.RequestNextGame(channel.SocketContext{UID: "w"})
		if drained(t, r) {
			t.Fatal("next-game accepted while the game was still undecided")
		}
	})

	t.Run("classic room", func(t *testing.T) {
		r := newTestInstance(t, "w", "b")
		r.controlChannel = make(chan message.RoomControl, 2)
		driveToOngoing(t, r)
		r.stateMu.Lock()
		r.game.Resign(octad.Black)
		r.stateMu.Unlock()

		r.RequestNextGame(channel.SocketContext{UID: "w"})
		if drained(t, r) {
			t.Fatal("next-game accepted in a room with no match to continue")
		}
	})

	t.Run("decided match", func(t *testing.T) {
		r := newTestInstance(t, "w", "b")
		r.params.RaceTo = 1
		r.controlChannel = make(chan message.RoomControl, 2)
		driveToOngoing(t, r)
		r.stateMu.Lock()
		r.game.Resign(octad.Black)
		r.updateScoreLocked() // 1-0 decides a race to 1
		r.stateMu.Unlock()

		r.RequestNextGame(channel.SocketContext{UID: "w"})
		if drained(t, r) {
			t.Fatal("next-game accepted after the race was decided")
		}
	})

	t.Run("non-player", func(t *testing.T) {
		r := newTestInstance(t, "w", "b")
		r.params.RaceTo = 3
		r.controlChannel = make(chan message.RoomControl, 2)
		driveToOngoing(t, r)
		r.stateMu.Lock()
		r.game.Resign(octad.Black)
		r.updateScoreLocked()
		r.stateMu.Unlock()

		r.RequestNextGame(channel.SocketContext{UID: "spectator"})
		if drained(t, r) {
			t.Fatal("next-game accepted from a viewer holding no seat")
		}
	})

	t.Run("decided outcome sliver", func(t *testing.T) {
		r := newTestInstance(t, "w", "b")
		r.params.RaceTo = 3
		r.controlChannel = make(chan message.RoomControl, 2)
		driveToOngoing(t, r)
		r.stateMu.Lock()
		r.game.Resign(octad.Black)
		r.updateScoreLocked()
		r.stateMu.Unlock()
		if r.State() != StateGameOngoing {
			t.Fatalf("precondition: expected StateGameOngoing, got %s", r.State())
		}

		r.RequestNextGame(channel.SocketContext{UID: "b"})
		select {
		case ctrl := <-r.controlChannel:
			if ctrl.Type != message.NextGame {
				t.Fatalf("expected NextGame control, got %v", ctrl.Type)
			}
		default:
			t.Fatal("next-game dropped in the decided-but-not-transitioned sliver")
		}
	})
}

// TestMatchInterludeBothReadyStartsEarly drives the interlude with a deadline
// far in the future: both players asking for the next game must start it
// without waiting the pause out.
func TestMatchInterludeBothReadyStartsEarly(t *testing.T) {
	prev := nextGameReadyBeat
	nextGameReadyBeat = 5 * time.Millisecond
	defer func() { nextGameReadyBeat = prev }()

	r := newTestInstance(t, "w", "b")
	r.params.RaceTo = 3
	r.controlChannel = make(chan message.RoomControl, 2)
	driveToGameOver(t, r)

	r.stateMu.Lock()
	r.updateScoreLocked()
	// a deadline the test can never reach, so only the skip can advance us
	r.nextGameDeadline = time.Now().Add(time.Minute)
	finished := r.game.ID
	r.stateMu.Unlock()

	done := make(chan struct{})
	go func() {
		r.handleMatchInterlude()
		close(done)
	}()

	sendNextGame(r, "w")
	sendNextGame(r, "b")

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("both players ready did not start the next game")
	}

	if r.State() != StateGameReady {
		t.Fatalf("expected StateGameReady after the skip, got %s", r.State())
	}
	r.stateMu.Lock()
	next := r.game.ID
	readyW := r.nextGame.AgreedBy(octad.White)
	readyB := r.nextGame.AgreedBy(octad.Black)
	r.stateMu.Unlock()
	if next == finished {
		t.Fatal("the next game was not built")
	}
	if readyW || readyB {
		t.Fatal("interlude readiness survived into the next game")
	}
}

// TestMatchInterludeOneReadyWaits asserts a single player's request does not
// short-circuit the pause: the interlude still runs to its deadline, which is
// what keeps "skip" a two-sided handshake rather than one player rushing the
// other off the result card.
func TestMatchInterludeOneReadyWaits(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.params.RaceTo = 3
	r.controlChannel = make(chan message.RoomControl, 2)
	driveToGameOver(t, r)

	r.stateMu.Lock()
	r.updateScoreLocked()
	r.nextGameDeadline = time.Now().Add(150 * time.Millisecond)
	r.stateMu.Unlock()

	done := make(chan struct{})
	go func() {
		r.handleMatchInterlude()
		close(done)
	}()

	sendNextGame(r, "w")

	// the lone request must be recorded (so the opponent's client can show the
	// check) without advancing before the deadline
	select {
	case <-done:
		t.Fatal("one player's request started the next game early")
	case <-time.After(60 * time.Millisecond):
	}

	r.stateMu.Lock()
	readyW := r.nextGame.AgreedBy(octad.White)
	readyB := r.nextGame.AgreedBy(octad.Black)
	r.stateMu.Unlock()
	if !readyW || readyB {
		t.Fatalf("recorded readiness = w:%v b:%v, want only white", readyW, readyB)
	}

	// the deadline still decides; both seats are disconnected in this test, so
	// the handler holds its forfeit grace instead of advancing — the point here
	// is only that the lone request did not advance it
	select {
	case <-done:
		t.Fatal("interlude ended without both players connected")
	case <-time.After(200 * time.Millisecond):
	}
	close(r.done)
	<-time.After(20 * time.Millisecond)
}

// TestMatchInterludeIgnoresRematchControl asserts a stray rematch click (which
// RequestRematch's decided-outcome window accepts) is swallowed mid-match and
// never mistaken for interlude readiness.
func TestMatchInterludeIgnoresRematchControl(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.params.RaceTo = 3
	r.controlChannel = make(chan message.RoomControl, 2)
	driveToGameOver(t, r)

	r.stateMu.Lock()
	r.updateScoreLocked()
	r.nextGameDeadline = time.Now().Add(time.Minute)
	r.stateMu.Unlock()

	done := make(chan struct{})
	go func() {
		r.handleMatchInterlude()
		close(done)
	}()

	r.controlChannel <- message.RoomControl{
		Type: message.Rematch,
		Ctx:  channel.SocketContext{UID: "w"},
	}
	r.controlChannel <- message.RoomControl{
		Type: message.Rematch,
		Ctx:  channel.SocketContext{UID: "b"},
	}

	select {
	case <-done:
		t.Fatal("rematch clicks advanced an undecided match's interlude")
	case <-time.After(100 * time.Millisecond):
	}

	r.stateMu.Lock()
	readyW := r.nextGame.AgreedBy(octad.White)
	readyB := r.nextGame.AgreedBy(octad.Black)
	r.stateMu.Unlock()
	if readyW || readyB {
		t.Fatal("a rematch control was recorded as interlude readiness")
	}

	close(r.done)
	<-time.After(20 * time.Millisecond)
}
