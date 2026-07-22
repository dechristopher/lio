package room

import (
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"
	"github.com/looplab/fsm"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
)

// applyResults replays a shorthand sequence of game results onto the room's
// players: 'w' = white wins, 'b' = black wins, 'd' = draw. Seats are not
// flipped between games (matchDecidedLocked only reads accumulated points, and
// the score follows the player through flips anyway).
func applyResults(r *Instance, results string) {
	for _, c := range results {
		switch c {
		case 'w':
			r.players.ScoreWin(octad.White, "checkmate")
		case 'b':
			r.players.ScoreWin(octad.Black, "checkmate")
		case 'd':
			r.players.ScoreDraw("stalemate")
		}
	}
}

// driveToGameOver advances a fresh instance's FSM into StateGameOver.
func driveToGameOver(t *testing.T, r *Instance) {
	t.Helper()
	for _, ev := range []fsm.EventDesc{
		EventRoomInitialized, EventPlayersConnected,
		EventStartGame, EventWhiteWinsResignation,
	} {
		if err := r.event(ev); err != nil {
			t.Fatalf("event %s: %v", ev.Name, err)
		}
	}
	if r.State() != StateGameOver {
		t.Fatalf("expected StateGameOver, got %s", r.State())
	}
}

// TestMatchDecided locks the race-to decision rule: a seat wins the match once
// its score reaches RaceTo points AND strictly leads the opponent. A tied
// arrival at the target decides nothing (win-by-lead continuation), and a
// non-match room (RaceTo 0) is never decided.
func TestMatchDecided(t *testing.T) {
	cases := []struct {
		name    string
		raceTo  int
		results string
		decided bool
		winner  octad.Color
	}{
		{"no games", 2, "", false, octad.NoColor},
		{"mid race", 2, "w", false, octad.NoColor},
		{"white sweeps", 2, "ww", true, octad.White},
		{"black sweeps", 2, "bb", true, octad.Black},
		{"tied arrival at target", 2, "dddd", false, octad.NoColor},
		{"lead after tied arrival", 2, "ddddw", true, octad.White},
		{"draw lifts leader to target", 2, "wdd", true, octad.White},
		{"draws alone never lead", 2, "dddddd", false, octad.NoColor},
		{"decisive but short of target", 3, "wbw", false, octad.NoColor},
		{"non-match room", 0, "ww", false, octad.NoColor},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newTestInstance(t, "wp", "bp")
			r.params.RaceTo = tc.raceTo
			applyResults(r, tc.results)

			decided, winner := r.MatchDecided()
			if decided != tc.decided || winner != tc.winner {
				t.Fatalf("MatchDecided() = (%v, %v), want (%v, %v)",
					decided, winner, tc.decided, tc.winner)
			}
		})
	}
}

// TestMatchGameOverPayloadMidMatch asserts an undecided match's game-over
// payload advertises the race (rt), no rematch window, and the auto-advance
// countdown (ng) at the full interlude length.
func TestMatchGameOverPayloadMidMatch(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.params.RaceTo = 3

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.updateScoreLocked()
	payload := r.gameOverMessageLocked(false, "")
	r.stateMu.Unlock()

	msg := parseGameOver(t, payload)
	if msg.Data.RaceTo != 3 {
		t.Errorf("rt = %d, want 3", msg.Data.RaceTo)
	}
	if msg.Data.MatchOver {
		t.Error("a 1-0 game in a race to 3 must not read as match over")
	}
	if want := int(matchInterludeWindow.Seconds()); msg.Data.NextGameIn != want {
		t.Errorf("ng = %d, want %d", msg.Data.NextGameIn, want)
	}
	if msg.Data.RematchWindow != 0 {
		t.Errorf("mid-match game-over must carry no rematch window, got rw=%d",
			msg.Data.RematchWindow)
	}
}

// TestMatchGameOverPayloadMatchOver asserts a race-deciding game-over flips to
// the match-over shape: mo set, the usual rematch ("new match") window, and no
// auto-advance countdown.
func TestMatchGameOverPayloadMatchOver(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.params.RaceTo = 1

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.updateScoreLocked()
	payload := r.gameOverMessageLocked(false, "")
	r.stateMu.Unlock()

	msg := parseGameOver(t, payload)
	if msg.Data.RaceTo != 1 || !msg.Data.MatchOver {
		t.Errorf("rt/mo = %d/%v, want 1/true", msg.Data.RaceTo, msg.Data.MatchOver)
	}
	if want := int(rematchWindow.Seconds()); msg.Data.RematchWindow != want {
		t.Errorf("rw = %d, want %d (new-match window)", msg.Data.RematchWindow, want)
	}
	if msg.Data.NextGameIn != 0 {
		t.Errorf("a decided match must not auto-advance, got ng=%d", msg.Data.NextGameIn)
	}
}

// TestClassicGameOverPayloadNoMatchFields asserts a RaceTo=0 room's game-over
// payload is untouched by the match machinery: today's rematch window, none of
// the match fields.
func TestClassicGameOverPayloadNoMatchFields(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.updateScoreLocked()
	payload := r.gameOverMessageLocked(false, "")
	r.stateMu.Unlock()

	msg := parseGameOver(t, payload)
	if msg.Data.RaceTo != 0 || msg.Data.MatchOver || msg.Data.NextGameIn != 0 {
		t.Errorf("classic room leaked match fields: rt=%d mo=%v ng=%d",
			msg.Data.RaceTo, msg.Data.MatchOver, msg.Data.NextGameIn)
	}
	if want := int(rematchWindow.Seconds()); msg.Data.RematchWindow != want {
		t.Errorf("rw = %d, want %d", msg.Data.RematchWindow, want)
	}
}

// TestGameOverStateMessageMidMatchInterlude asserts a client (re)connecting
// during the interlude gets the remaining auto-advance countdown, not a rematch
// window — the reconnect analogue of the live mid-match broadcast.
func TestGameOverStateMessageMidMatchInterlude(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.params.RaceTo = 2

	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.updateScoreLocked()
	r.nextGameDeadline = time.Now().Add(4 * time.Second)
	r.stateMu.Unlock()

	msg := parseGameOver(t, r.GameOverStateMessage())
	if msg.Data.NextGameIn < 3 || msg.Data.NextGameIn > 4 {
		t.Errorf("ng = %d, want ~4 (remaining interlude)", msg.Data.NextGameIn)
	}
	if msg.Data.RematchWindow != 0 {
		t.Errorf("interlude reconnect must carry no rematch window, got rw=%d",
			msg.Data.RematchWindow)
	}
	if msg.Data.RaceTo != 2 || msg.Data.MatchOver {
		t.Errorf("rt/mo = %d/%v, want 2/false", msg.Data.RaceTo, msg.Data.MatchOver)
	}
}

// TestResetForNextGame locks the shared game-boundary reset: the game is
// swapped and per-game state cleared, with the accumulated score preserved
// between games of a match (resetScore=false) and cleared for a fresh match
// (resetScore=true).
func TestResetForNextGame(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.params.RaceTo = 2
	applyResults(r, "w") // 1-0 to the player currently seated white

	r.stateMu.Lock()
	r.nextGameDeadline = time.Now().Add(time.Minute)
	r.rematchDeadline = time.Now().Add(time.Minute)
	oldGameID := r.game.ID
	err := r.resetForNextGameLocked(false)
	r.stateMu.Unlock()
	if err != nil {
		t.Fatalf("resetForNextGameLocked: %v", err)
	}

	r.stateMu.Lock()
	newGameID := r.game.ID
	score := r.players.ScoreMap()
	deadlinesCleared := r.nextGameDeadline.IsZero() && r.rematchDeadline.IsZero()
	r.stateMu.Unlock()

	if newGameID == oldGameID {
		t.Error("expected a fresh game after the reset")
	}
	if !deadlinesCleared {
		t.Error("published game-over deadlines must clear at the game boundary")
	}
	// the board flipped, so the winner's point followed them to the black seat
	if score["w"] != 0 || score["b"] != 1 {
		t.Errorf("score after flip = w:%v b:%v, want w:0 b:1 (preserved)",
			score["w"], score["b"])
	}

	// a fresh match clears the score and history for both seats
	r.stateMu.Lock()
	err = r.resetForNextGameLocked(true)
	score = r.players.ScoreMap()
	histLen := len(r.players.MatchHistory())
	r.stateMu.Unlock()
	if err != nil {
		t.Fatalf("resetForNextGameLocked(reset): %v", err)
	}
	if score["w"] != 0 || score["b"] != 0 || histLen != 0 {
		t.Errorf("new match must reset scores/history, got w:%v b:%v hist:%d",
			score["w"], score["b"], histLen)
	}
}

// TestMatchInterludeAdvances drives the auto-advance path end to end: an
// undecided match's game-over runs the interlude and starts the next game with
// no rematch agreement, preserving the score and sweeping any stray rematch
// click buffered during the finished game.
func TestMatchInterludeAdvances(t *testing.T) {
	prevWindow := matchInterludeWindow
	matchInterludeWindow = 50 * time.Millisecond
	defer func() { matchInterludeWindow = prevWindow }()

	r := newTestInstance(t, "wp", "bp")
	r.ID = "matchtest-advance" // unique channel in the global sockmap
	r.params.RaceTo = 2
	r.controlChannel = make(chan message.RoomControl, 2)

	sm := channel.Map.GetSockMap(r.ID)
	defer sm.Cleanup()
	sm.Track(channel.NewSocket(nil, "wp", "c1", ""))
	sm.Track(channel.NewSocket(nil, "bp", "c1", ""))

	driveToGameOver(t, r)
	applyResults(r, "w") // 1-0: match undecided

	// a stray click accepted by RequestRematch's decided-outcome window must be
	// swallowed at the boundary, never replayed as a later agreement
	r.controlChannel <- message.RoomControl{
		Type: message.Rematch,
		Ctx:  channel.SocketContext{Channel: r.ID, UID: "wp"},
	}

	done := make(chan struct{})
	go func() {
		r.handleGameOver()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("match interlude did not advance in time")
	}

	if r.State() != StateGameReady {
		t.Fatalf("expected StateGameReady after auto-advance, got %s", r.State())
	}
	if len(r.controlChannel) != 0 {
		t.Error("stray rematch click survived the game boundary")
	}

	r.stateMu.Lock()
	score := r.players.ScoreMap()
	agreed := r.rematch.Agreed()
	r.stateMu.Unlock()
	// the board flipped between games: the leader now sits black
	if score["w"] != 0 || score["b"] != 1 {
		t.Errorf("score after advance = w:%v b:%v, want w:0 b:1 (preserved)",
			score["w"], score["b"])
	}
	if agreed {
		t.Error("auto-advance must not record a rematch agreement")
	}
}

// TestMatchInterludeForfeit asserts a player still missing after the interlude
// and its disconnect grace forfeits the match: the room is abandoned rather
// than grinding out games against an empty seat.
func TestMatchInterludeForfeit(t *testing.T) {
	prevWindow, prevGrace := matchInterludeWindow, rematchDisconnectGrace
	matchInterludeWindow = 50 * time.Millisecond
	rematchDisconnectGrace = 50 * time.Millisecond
	defer func() {
		matchInterludeWindow, rematchDisconnectGrace = prevWindow, prevGrace
	}()

	r := newTestInstance(t, "wp", "bp")
	r.ID = "matchtest-forfeit"
	r.params.RaceTo = 2
	r.controlChannel = make(chan message.RoomControl, 2)

	// sockmap exists but nobody is connected
	sm := channel.Map.GetSockMap(r.ID)
	defer sm.Cleanup()

	driveToGameOver(t, r)
	applyResults(r, "w")

	done := make(chan struct{})
	go func() {
		r.handleGameOver()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("interlude did not forfeit in time")
	}

	if r.State() != StateRoomOver {
		t.Fatalf("expected StateRoomOver after forfeit, got %s", r.State())
	}
	if !r.abandoned {
		t.Error("a forfeited match must mark the room abandoned")
	}
}

// TestNewMatchAgreementResetsScores drives a decided match through the "new
// match" window: both players' rematch clicks restart the race in the same
// room with scores and history cleared.
func TestNewMatchAgreementResetsScores(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.ID = "matchtest-newmatch"
	r.params.RaceTo = 1
	r.controlChannel = make(chan message.RoomControl, 2)

	sm := channel.Map.GetSockMap(r.ID)
	defer sm.Cleanup()
	sm.Track(channel.NewSocket(nil, "wp", "c1", ""))
	sm.Track(channel.NewSocket(nil, "bp", "c1", ""))

	driveToGameOver(t, r)
	applyResults(r, "w") // 1-0 in a race to 1: match decided

	// both seats agree to a new match; buffered, consumed once the window opens
	for _, uid := range []string{"wp", "bp"} {
		r.controlChannel <- message.RoomControl{
			Type: message.Rematch,
			Ctx:  channel.SocketContext{Channel: r.ID, UID: uid},
		}
	}

	done := make(chan struct{})
	go func() {
		r.handleGameOver()
		close(done)
	}()

	select {
	case <-done:
	// the agreement path holds a deliberate 1s pause before the next game
	case <-time.After(5 * time.Second):
		t.Fatal("new-match agreement did not restart in time")
	}

	if r.State() != StateGameReady {
		t.Fatalf("expected StateGameReady after new-match agreement, got %s", r.State())
	}

	r.stateMu.Lock()
	score := r.players.ScoreMap()
	histLen := len(r.players.MatchHistory())
	r.stateMu.Unlock()
	if score["w"] != 0 || score["b"] != 0 || histLen != 0 {
		t.Errorf("new match must start 0-0 with empty history, got w:%v b:%v hist:%d",
			score["w"], score["b"], histLen)
	}
}
