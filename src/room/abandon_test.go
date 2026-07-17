package room

import (
	"testing"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
)

// TestHumanMovedThisGame covers the engagement flag that gates bot-game
// auto-rematch: a game is only auto-rematched once the human has actually moved.
func TestHumanMovedThisGame(t *testing.T) {
	r := newBotTestInstance(t, "human", octad.White)

	if r.humanMovedThisGame() {
		t.Fatal("a fresh game should report no human move")
	}

	r.humanMoved = true
	if !r.humanMovedThisGame() {
		t.Fatal("should report a human move once one is recorded")
	}
}

// TestHumanIdleEligible covers the condition under which a connected-but-idle
// human in a bot game is abandoned: a bot game, the human has not moved, and it
// is their turn. It must be false while it is the bot's turn (we never abandon a
// game waiting on the engine) and false once the human has engaged.
func TestHumanIdleEligible(t *testing.T) {
	// bot plays White, human plays Black
	r := newBotTestInstance(t, "human", octad.White)

	// fresh game: White (the bot) is to move, so it is not the human's turn
	if r.humanIdleEligible() {
		t.Fatal("idle-eligible while it is the bot's turn; want false")
	}

	// play the bot's opening move so it becomes the human's (Black) turn
	moves := r.game.ValidMoves()
	if len(moves) == 0 {
		t.Fatal("expected legal opening moves")
	}
	if err := r.game.Move(moves[0]); err != nil {
		t.Fatalf("apply opening move: %v", err)
	}
	r.game.ToMove = r.game.Position().Turn()

	// now the no-show human is on the clock with no move played → eligible
	if !r.humanIdleEligible() {
		t.Fatal("idle-eligible = false; want true when a no-show human is on the clock")
	}

	// once the human has engaged, never idle-eligible again this game
	r.humanMoved = true
	if r.humanIdleEligible() {
		t.Fatal("idle-eligible after the human moved; want false")
	}
}

// TestHumanIdleEligibleNonBotRoom asserts the idle-abandon never applies to a
// human-vs-human room, which has no bot color.
func TestHumanIdleEligibleNonBotRoom(t *testing.T) {
	r := newTestInstance(t, "creator", "opponent")
	if r.humanIdleEligible() {
		t.Fatal("a human-vs-human room must never be idle-eligible")
	}
}

// TestHumanIdleEligibleRunningClock covers the flag-over-abandon carve-out: a
// connected no-show human whose clock is genuinely running is left to lose on
// time (post pre-start-countdown deploy games, classic games where the bot
// moved first), while a disconnected human or a paused clock stays eligible.
func TestHumanIdleEligibleRunningClock(t *testing.T) {
	// bot plays White, human plays Black; put the human on the clock
	r := newBotTestInstance(t, "human", octad.White)
	moves := r.game.ValidMoves()
	if len(moves) == 0 {
		t.Fatal("expected legal opening moves")
	}
	if err := r.game.Move(moves[0]); err != nil {
		t.Fatalf("apply opening move: %v", err)
	}
	r.game.ToMove = r.game.Position().Turn()

	// paused clock, no connection: the pre-carve-out baseline — eligible
	if !r.humanIdleEligible() {
		t.Fatal("no-show human on a paused clock should be idle-eligible")
	}

	// running clock but still no connection: the abandon timer's territory,
	// and the idle timer stays armed as belt and braces — eligible
	r.game.Clock.Start()
	t.Cleanup(func() { r.game.Clock.Stop(false, true) })
	if !r.humanIdleEligible() {
		t.Fatal("disconnected human should stay idle-eligible, running clock or not")
	}

	// running clock AND connected: the flag governs — never idle-abandon
	sm := channel.Map.GetSockMap(r.ID)
	sm.Track(channel.NewSocket(nil, "human", "c1", ""))
	t.Cleanup(func() { sm.UnTrack("human", "c1") })
	if r.humanIdleEligible() {
		t.Fatal("connected human on a running clock must flag, not idle-abandon")
	}

	// paused clock while connected (a restored room awaiting resume): eligible
	r.game.Clock.Stop(false, true)
	if !r.humanIdleEligible() {
		t.Fatal("connected human on a paused clock should be idle-eligible")
	}
}
