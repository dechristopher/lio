package room

import (
	"testing"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
)

// drainControl returns the next buffered control (or fails if none), for
// asserting what RequestResign / RequestDraw enqueued.
func drainControl(t *testing.T, r *Instance) (message.RoomControl, bool) {
	t.Helper()
	select {
	case c := <-r.controlChannel:
		return c, true
	default:
		return message.RoomControl{}, false
	}
}

// TestRequestResignEnqueues covers the resign control's guards: a seated player
// resigning during an ongoing, undecided game enqueues a Resign control; a
// non-player and a request outside the ongoing state are dropped.
func TestRequestResignEnqueues(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.controlChannel = make(chan message.RoomControl, 2)

	// before the game starts (StateInit) a resign must be dropped
	r.RequestResign(channel.SocketContext{UID: "w"})
	if _, ok := drainControl(t, r); ok {
		t.Fatal("resign accepted before the game was ongoing")
	}

	driveToOngoing(t, r)

	// a spectator (non-seated uid) must be dropped
	r.RequestResign(channel.SocketContext{UID: "spectator"})
	if _, ok := drainControl(t, r); ok {
		t.Fatal("resign accepted from a non-player")
	}

	// a seated player's resign is enqueued as a Resign control
	r.RequestResign(channel.SocketContext{UID: "w"})
	ctrl, ok := drainControl(t, r)
	if !ok {
		t.Fatal("seated player's resign was dropped")
	}
	if ctrl.Type != message.Resign {
		t.Fatalf("control type = %v, want Resign", ctrl.Type)
	}

	// once the game is decided, further resigns are dropped
	r.stateMu.Lock()
	r.game.Resign(octad.Black)
	r.stateMu.Unlock()
	r.RequestResign(channel.SocketContext{UID: "w"})
	if _, ok := drainControl(t, r); ok {
		t.Fatal("resign accepted after the game was decided")
	}
}

// TestRequestDrawEnqueues covers the draw control's guards, mirroring resign.
func TestRequestDrawEnqueues(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	r.controlChannel = make(chan message.RoomControl, 2)

	r.RequestDraw(channel.SocketContext{UID: "w"})
	if _, ok := drainControl(t, r); ok {
		t.Fatal("draw accepted before the game was ongoing")
	}

	driveToOngoing(t, r)

	r.RequestDraw(channel.SocketContext{UID: "spectator"})
	if _, ok := drainControl(t, r); ok {
		t.Fatal("draw accepted from a non-player")
	}

	r.RequestDraw(channel.SocketContext{UID: "w"})
	ctrl, ok := drainControl(t, r)
	if !ok {
		t.Fatal("seated player's draw was dropped")
	}
	if ctrl.Type != message.Draw {
		t.Fatalf("control type = %v, want Draw", ctrl.Type)
	}
}

// TestDrawControlStandingOffer verifies a single human draw offer records the
// offering side and becomes the standing offer without ending the game, and that
// re-offering from the same side is an idempotent no-op.
func TestDrawControlStandingOffer(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	driveToOngoing(t, r)

	over, _ := r.drawControl(message.RoomControl{
		Type: message.Draw,
		Ctx:  channel.SocketContext{UID: "w"},
	})
	if over {
		t.Fatal("a single draw offer must not end the game")
	}
	if r.drawOffer != octad.White {
		t.Fatalf("drawOffer = %v, want White", r.drawOffer)
	}
	if !r.draw[octad.White] || r.draw[octad.Black] {
		t.Fatalf("draw agreement = %v, want only White marked", r.draw)
	}

	// re-offering from the same side is a no-op and leaves the standing offer
	over, _ = r.drawControl(message.RoomControl{
		Type: message.Draw,
		Ctx:  channel.SocketContext{UID: "w"},
	})
	if over || r.drawOffer != octad.White {
		t.Fatalf("re-offer changed state: over=%v drawOffer=%v", over, r.drawOffer)
	}
}

// TestDrawControlRejectsNonPlayer confirms a draw control from a non-seated uid
// records nothing and never ends the game.
func TestDrawControlRejectsNonPlayer(t *testing.T) {
	r := newTestInstance(t, "w", "b")
	driveToOngoing(t, r)

	over, _ := r.drawControl(message.RoomControl{
		Type: message.Draw,
		Ctx:  channel.SocketContext{UID: "spectator"},
	})
	if over {
		t.Fatal("a non-player draw control must not end the game")
	}
	if r.drawOffer != octad.NoColor {
		t.Fatalf("drawOffer = %v, want NoColor after a non-player control", r.drawOffer)
	}
}

// TestHandleDrawEvalDeclineClearsOffer verifies the bot-decline path: a fresh
// verdict declining a standing offer clears it and does not end the game.
func TestHandleDrawEvalDeclineClearsOffer(t *testing.T) {
	r := newBotTestInstance(t, "human", octad.White) // bot White, human Black
	driveToOngoing(t, r)

	// seed a standing offer from the human (Black)
	r.draw = player.NewAgreement()
	r.draw.Agree(octad.Black)
	r.drawOffer = octad.Black

	over, _ := r.handleDrawEval(&message.RoomDrawEval{
		GameID: r.game.ID,
		OFEN:   r.game.OFEN(),
		Accept: false,
	})
	if over {
		t.Fatal("a declined draw must not end the game")
	}
	if r.drawOffer != octad.NoColor {
		t.Fatalf("drawOffer = %v, want NoColor after a decline", r.drawOffer)
	}
	if r.draw.Agreed() {
		t.Fatal("draw agreement should be reset after a decline")
	}
}

// TestHandleDrawEvalStaleDropped verifies stale/no-longer-applicable verdicts are
// dropped without touching the standing offer or ending the game: a verdict for a
// prior game id, and a verdict arriving with no offer pending.
func TestHandleDrawEvalStaleDropped(t *testing.T) {
	r := newBotTestInstance(t, "human", octad.White)
	driveToOngoing(t, r)

	// a standing offer with a verdict tagged for a different game id is stale
	r.draw = player.NewAgreement()
	r.draw.Agree(octad.Black)
	r.drawOffer = octad.Black

	over, _ := r.handleDrawEval(&message.RoomDrawEval{
		GameID: "stale-game-id",
		OFEN:   r.game.OFEN(),
		Accept: true, // would end the game if not dropped
	})
	if over {
		t.Fatal("a stale-game-id verdict must be dropped, not applied")
	}
	if r.drawOffer != octad.Black {
		t.Fatalf("stale verdict altered the standing offer: drawOffer=%v", r.drawOffer)
	}

	// with no offer pending, even a fresh accepting verdict is dropped
	r.drawOffer = octad.NoColor
	r.draw = player.NewAgreement()
	over, _ = r.handleDrawEval(&message.RoomDrawEval{
		GameID: r.game.ID,
		OFEN:   r.game.OFEN(),
		Accept: true,
	})
	if over {
		t.Fatal("a verdict with no pending offer must be dropped")
	}
}
