package room

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/www/ws/proto"
)

// playTestMoves applies n legal moves through makeMove — the real move path
// (game mutation + clock flip + broadcasts) — alternating the acting seat.
// The room's clock must already be running.
func playTestMoves(t *testing.T, r *Instance, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		r.stateMu.Lock()
		moves := r.game.ValidMoves()
		uid := "wp"
		if r.game.ToMove == octad.Black {
			uid = "bp"
		}
		r.stateMu.Unlock()
		if len(moves) == 0 {
			t.Fatalf("no legal moves at ply %d", i)
		}
		ok := r.makeMove(&message.RoomMove{
			Move: proto.MovePayload{UOI: moves[0].String()},
			Ctx:  channel.SocketContext{Channel: r.ID, UID: uid, MT: 1},
		})
		if !ok {
			t.Fatalf("move %s rejected at ply %d", moves[0], i)
		}
	}
}

// TestPersistRoundTripOngoing locks snapshot fidelity for a live game: board,
// move history, game identity, scores + per-game match history, draw-offer
// state, engagement flag, variant definition (CTime JSON round trip), and the
// paused as-of-last-flip clock all survive Persist → Rehydrate, and the
// restored room is primed to resume (state + resumeClockPending).
func TestPersistRoundTripOngoing(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	driveToOngoing(t, r)
	r.game.Clock.Start()
	defer r.game.Clock.Stop(false, true)

	playTestMoves(t, r, 3)

	// accumulated match score from earlier games: white 1.5, black 0.5
	applyResults(r, "wd")

	// a standing draw offer from white
	r.stateMu.Lock()
	r.drawOffer = octad.White
	r.draw.Agree(octad.White)
	r.stateMu.Unlock()

	data, ok := r.Persist()
	if !ok {
		t.Fatal("ongoing room did not persist")
	}
	wantClock := r.game.Clock.Snapshot()

	r2, err := Rehydrate(data)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}

	if r2.State() != StateGameOngoing {
		t.Fatalf("restored state = %s, want %s", r2.State(), StateGameOngoing)
	}
	if !r2.resumeClockPending {
		t.Fatal("restored ongoing room must be pending a clock resume")
	}
	if r2.ID != r.ID || r2.creator != r.creator {
		t.Fatalf("room identity lost: %s/%s, want %s/%s", r2.ID, r2.creator, r.ID, r.creator)
	}
	if r2.game.ID != r.game.ID {
		t.Fatalf("game identity lost: %s, want %s", r2.game.ID, r.game.ID)
	}

	if got, want := r2.game.OFEN(), r.game.OFEN(); got != want {
		t.Fatalf("restored position = %s, want %s", got, want)
	}
	gotMoves, wantMoves := r2.game.MoveHistory(), r.game.MoveHistory()
	if len(gotMoves) != len(wantMoves) {
		t.Fatalf("restored %d moves, want %d", len(gotMoves), len(wantMoves))
	}
	for i := range wantMoves {
		if gotMoves[i] != wantMoves[i] {
			t.Fatalf("move %d = %s, want %s", i, gotMoves[i], wantMoves[i])
		}
	}

	if got := r2.players[octad.White].Score(); got != 1.5 {
		t.Fatalf("white score = %v, want 1.5", got)
	}
	if got := r2.players[octad.Black].Score(); got != 0.5 {
		t.Fatalf("black score = %v, want 0.5", got)
	}
	if got := len(r2.players[octad.White].Results()); got != 2 {
		t.Fatalf("white match history has %d games, want 2", got)
	}

	if r2.drawOffer != octad.White || !r2.draw.AgreedBy(octad.White) {
		t.Fatal("standing draw offer lost in round trip")
	}
	if !r2.humanMoved {
		t.Fatal("engagement flag (humanMoved) lost in round trip")
	}

	if got := r2.game.Clock.Snapshot(); got != wantClock {
		t.Fatalf("restored clock = %+v, want %+v", got, wantClock)
	}
	if !r2.game.Clock.State(true).IsPaused {
		t.Fatal("restored clock must be paused until the resume gate")
	}

	if got, want := r2.params.GameConfig.Variant.Control.Time.Centi(),
		r.params.GameConfig.Variant.Control.Time.Centi(); got != want {
		t.Fatalf("variant time control = %d, want %d (CTime JSON round trip)", got, want)
	}
}

// TestPersistSkipsWaitingRooms: open challenges are not persisted — a
// reconnecting client is redirected home instead (restore matrix decision).
func TestPersistSkipsWaitingRooms(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	if err := r.event(EventRoomInitialized); err != nil {
		t.Fatal(err)
	}
	if _, ok := r.Persist(); ok {
		t.Fatal("waiting_for_players room must not persist")
	}
}

// TestPersistGameOverResignation covers declared (non-board-derived) outcomes:
// a resignation does not re-arise from replaying the moves, so the snapshot's
// outcome is re-applied to the rebuilt game. The restored room re-enters the
// game-over window with the remaining persisted window.
func TestPersistGameOverResignation(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	driveToOngoing(t, r)
	r.game.Clock.Start()
	playTestMoves(t, r, 2)

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.updateScoreLocked()
	r.rematchDeadline = time.Now().Add(10 * time.Second)
	r.stateMu.Unlock()
	r.game.Clock.Stop(false, true)
	if err := r.event(EventWhiteWinsResignation); err != nil {
		t.Fatal(err)
	}

	data, ok := r.Persist()
	if !ok {
		t.Fatal("game-over room did not persist")
	}

	var p PersistedRoom
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.State != StateGameOver {
		t.Fatalf("persisted state = %s, want %s", p.State, StateGameOver)
	}

	r2, err := Rehydrate(data)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if r2.State() != StateGameOver {
		t.Fatalf("restored state = %s, want %s", r2.State(), StateGameOver)
	}
	if r2.game.Outcome() != octad.WhiteWon || r2.game.Method() != octad.Resignation {
		t.Fatalf("restored outcome = %s (%d), want white win by resignation",
			r2.game.Outcome(), r2.game.Method())
	}
	if got := r2.players[octad.White].Score(); got != 1 {
		t.Fatalf("white score = %v, want 1", got)
	}
	if r2.resumeClockPending {
		t.Fatal("a finished game must not be pending a clock resume")
	}
	if r2.restoredWindow < rematchDisconnectGrace || r2.restoredWindow > 10*time.Second+time.Second {
		t.Fatalf("restored window = %s, want ≈10s within [grace, full]", r2.restoredWindow)
	}
}

// TestPersistGameOverLapsedWindowFloorsAtGrace: a rematch window that expired
// while the process was down still gives returning players one disconnect
// grace before the room closes, and a restart can never extend a window past
// its full length.
func TestPersistGameOverLapsedWindowFloorsAtGrace(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	driveToOngoing(t, r)
	r.game.Clock.Start()
	playTestMoves(t, r, 2)

	r.stateMu.Lock()
	r.game.Resign(octad.White)
	r.rematchDeadline = time.Now().Add(-time.Minute)
	r.stateMu.Unlock()
	r.game.Clock.Stop(false, true)
	if err := r.event(EventBlackWinsResignation); err != nil {
		t.Fatal(err)
	}

	data, ok := r.Persist()
	if !ok {
		t.Fatal("game-over room did not persist")
	}
	r2, err := Rehydrate(data)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if r2.restoredWindow != rematchDisconnectGrace {
		t.Fatalf("lapsed window restored as %s, want the grace floor %s",
			r2.restoredWindow, rematchDisconnectGrace)
	}
}

// TestPersistDeployNormalizesToGameReady: a room captured mid-deploy persists
// at game_ready with no per-game state — the deploy phase re-runs from the top
// on restore (partial blind arrangements are deliberately dropped), while the
// room-level state (seats, params, scores) survives.
func TestPersistDeployNormalizesToGameReady(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.params.Deploy = true
	driveToDeploy(t, r)

	data, ok := r.Persist()
	if !ok {
		t.Fatal("deploy-phase room did not persist")
	}

	var p PersistedRoom
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.State != StateGameReady {
		t.Fatalf("persisted state = %s, want %s", p.State, StateGameReady)
	}
	if len(p.Moves) != 0 || p.GameID != "" {
		t.Fatal("game_ready snapshot must carry no per-game state")
	}

	r2, err := Rehydrate(data)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if r2.State() != StateGameReady {
		t.Fatalf("restored state = %s, want %s", r2.State(), StateGameReady)
	}
	if r2.resumeClockPending {
		t.Fatal("a game_ready room must not be pending a clock resume")
	}
	if !r2.params.Deploy {
		t.Fatal("deploy param lost in round trip")
	}
	cs := r2.game.Clock.State(true)
	if !cs.IsPaused {
		t.Fatal("fresh restored game must have a paused clock")
	}
}

// TestRehydratedRoomResumesOnReconnect drives the full resume gate: a
// rehydrated ongoing room starts its routine with the clock paused, and the
// clock resumes — without charging the downtime — once both seats hold live
// connections again.
func TestRehydratedRoomResumesOnReconnect(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.ID = "resumeroom" // unique registry key for StartRehydrated
	driveToOngoing(t, r)
	r.game.Clock.Start()
	playTestMoves(t, r, 2)

	data, ok := r.Persist()
	if !ok {
		t.Fatal("ongoing room did not persist")
	}
	r.game.Clock.Stop(false, true)
	persisted := r.game.Clock.Snapshot()

	r2, err := Rehydrate(data)
	if err != nil {
		t.Fatalf("rehydrate: %v", err)
	}
	if err := r2.StartRehydrated(); err != nil {
		t.Fatalf("start rehydrated: %v", err)
	}
	t.Cleanup(func() { rooms.Delete(r2.ID) })

	if !r2.game.Clock.State(true).IsPaused {
		t.Fatal("restored clock must be paused before players reconnect")
	}

	// both players reconnect
	sm := channel.Map.GetSockMap(r2.ID)
	sm.Track(channel.NewSocket(nil, "wp", "c1", ""))
	sm.Track(channel.NewSocket(nil, "bp", "c1", ""))
	t.Cleanup(func() {
		sm.UnTrack("wp", "c1")
		sm.UnTrack("bp", "c1")
	})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if state := r2.game.Clock.State(true); !state.IsPaused {
			// resumed: the mover's remaining time is the persisted remaining,
			// not persisted-minus-downtime
			rem := time.Duration(state.WhiteTime.Milli()+state.BlackTime.Milli()) * time.Millisecond
			want := 2*time.Duration(variantControlMs(r2))*time.Millisecond -
				time.Duration(persisted.WhiteElapsedMs+persisted.BlackElapsedMs)*time.Millisecond
			if diff := want - rem; diff < -time.Second || diff > time.Second {
				t.Fatalf("resumed remaining %s, want ≈%s (downtime must not be charged)", rem, want)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("clock never resumed after both players reconnected")
}

// variantControlMs returns the room variant's base time in milliseconds.
func variantControlMs(r *Instance) int64 {
	return r.params.GameConfig.Variant.Control.Time.Milli()
}
