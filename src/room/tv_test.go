package room

import (
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/engine"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/title"
	"github.com/dechristopher/lio/tv"
)

// TestTVSeat locks the identity the home-page TV grid shows for each kind of
// seat. The rule that matters is that nothing here is viewer-relative: the TV
// stream broadcasts one card to everyone, so an anonymous seat must resolve to
// a name a stranger can read rather than the room view's "You" (which no
// viewer of the grid could interpret).
func TestTVSeat(t *testing.T) {
	pawn := engine.PersonaByKey("pawn")

	tests := []struct {
		name  string
		seat  *player.Player
		perso string
		want  string
		bot   bool
		title string
		glyph string
	}{
		{
			name: "logged-in account carries its username and title badge",
			seat: &player.Player{
				ID:       "u1",
				Username: "drewtest",
				Title:    title.Title{Code: "GM", Name: "Grandmaster"},
			},
			want:  "drewtest",
			title: "GM",
		},
		{
			name: "untitled account shows a name and no badge",
			seat: &player.Player{ID: "u2", Username: "cdpplayer"},
			want: "cdpplayer",
		},
		{
			name: "anonymous human is spelled out, never left blank",
			seat: &player.Player{ID: "u3"},
			want: "Anonymous",
		},
		{
			name:  "bot seat is named by its difficulty persona",
			seat:  &player.Player{IsBot: true},
			perso: "pawn",
			want:  pawn.Name,
			bot:   true,
			glyph: pawn.Glyph,
		},
		{
			// every pre-persona room stamped no key; PersonaByKey resolves those
			// to the strongest persona, so the seat still gets a real label
			name:  "bot seat with no persona key falls back to a named persona",
			seat:  &player.Player{IsBot: true},
			perso: "",
			want:  engine.PersonaByKey("").Name,
			bot:   true,
			glyph: engine.PersonaByKey("").Glyph,
		},
		{
			name: "missing seat degrades to Anonymous rather than an empty card",
			seat: nil,
			want: "Anonymous",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestInstance(t, "wp", "bp")
			r.players[octad.White] = tt.seat
			r.params.BotPersona = tt.perso

			got := r.tvSeatLocked(octad.White)
			if got.Name != tt.want {
				t.Errorf("Name = %q, want %q", got.Name, tt.want)
			}
			if got.Bot != tt.bot {
				t.Errorf("Bot = %t, want %t", got.Bot, tt.bot)
			}
			if got.Title != tt.title {
				t.Errorf("Title = %q, want %q", got.Title, tt.title)
			}
			if got.Glyph != tt.glyph {
				t.Errorf("Glyph = %q, want %q", got.Glyph, tt.glyph)
			}
		})
	}
}

// TestTVEventCarriesBothSeats guards the wiring rather than the resolution: a
// TV event must describe both seats, since the grid renders a name above and
// below every board.
func TestTVEventCarriesBothSeats(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.players[octad.White] = &player.Player{ID: "wp", Username: "drewtest"}
	r.players[octad.Black] = &player.Player{IsBot: true}
	r.params.BotPersona = "knight"

	ev := r.tvEvent(tv.Start)
	if ev.WhiteSeat.Name != "drewtest" {
		t.Errorf("white seat = %q, want drewtest", ev.WhiteSeat.Name)
	}
	if !ev.BlackSeat.Bot || ev.BlackSeat.Name != engine.PersonaByKey("knight").Name {
		t.Errorf("black seat = %+v, want the knight persona", ev.BlackSeat)
	}
}

// TestTVEventDeployPhase covers what the grid needs to render a room that is
// still deploying: the phase flag that raises the home-rank covers, the shared
// countdown feeding the dial, per-seat confirmation, and — the point of it all —
// clocks reported as *not* running, since nothing is being charged yet.
func TestTVEventDeployPhase(t *testing.T) {
	r := newTestInstance(t, "white", "black")
	driveToDeploy(t, r)

	done := make(chan struct{})
	go func() {
		r.handleDeploy()
		close(done)
	}()
	t.Cleanup(func() {
		// let the deploy timer expire on its own if the test never completes it
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	})

	// one side in, one side still choosing — the state the grid most needs to
	// distinguish, and the one the "locked in" indicator exists for
	submitDeploy(r, "white", "knpp")
	waitForLock(t, r, octad.White)

	ev := r.tvEvent(tv.Deploy)
	if !ev.Deploying {
		t.Error("Deploying = false during the deploy phase")
	}
	if ev.Running {
		t.Error("Running = true during the deploy phase; no clock is being charged yet")
	}
	if ev.PhaseLeft <= 0 || ev.PhaseLeft > clock.ToCTime(deployTimeout).Centi() {
		t.Errorf("PhaseLeft = %d, want (0, %d]", ev.PhaseLeft, clock.ToCTime(deployTimeout).Centi())
	}
	if want := clock.ToCTime(deployTimeout).Centi(); ev.PhaseTotal != want {
		t.Errorf("PhaseTotal = %d, want %d", ev.PhaseTotal, want)
	}
	if !ev.WhiteSeat.Locked {
		t.Error("white committed its arrangement but the seat reads unlocked")
	}
	if ev.BlackSeat.Locked {
		t.Error("black has not committed but the seat reads locked")
	}

	submitDeploy(r, "black", "nkpp")
	<-done
	defer r.game.Clock.Stop(false, true)
}

// TestTVEventPreStartHoldsClocks is the regression for the grid draining White
// through the post-reveal countdown: the clock is unpaused for the whole grace
// but charges nobody, so the event must report it as not running (and hand the
// grid the countdown instead). Once the grace lapses the clocks are live again.
func TestTVEventPreStartHoldsClocks(t *testing.T) {
	r := newTestInstance(t, "white", "black")
	r.params.GameConfig.Variant.Control.PreStart = clock.ToCTime(300 * time.Millisecond)
	driveToDeploy(t, r)
	runDeployToCompletion(t, r)
	defer r.game.Clock.Stop(false, true)

	ev := r.tvEvent(tv.Start)
	if ev.Deploying {
		t.Error("Deploying = true after the reveal")
	}
	if ev.PhaseLeft <= 0 {
		t.Fatalf("PhaseLeft = %d, want a running pre-start countdown", ev.PhaseLeft)
	}
	if want := clock.ToCTime(300 * time.Millisecond).Centi(); ev.PhaseTotal != want {
		t.Errorf("PhaseTotal = %d, want %d", ev.PhaseTotal, want)
	}
	if ev.Running {
		t.Error("Running = true during the pre-start grace; White's clock is not draining yet")
	}

	// the grace lapses and the side to move goes on the clock for real
	time.Sleep(400 * time.Millisecond)
	if after := r.tvEvent(tv.Move); !after.Running || after.PhaseLeft != 0 {
		t.Errorf("after the grace: Running = %t, PhaseLeft = %d; want true, 0",
			after.Running, after.PhaseLeft)
	}
}

// waitForLock blocks until the deploy handler has recorded a color's committed
// arrangement (SubmitDeploy is asynchronous — it hands the room goroutine a
// message rather than applying it inline).
func waitForLock(t *testing.T, r *Instance, color octad.Color) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		r.stateMu.Lock()
		_, ok := r.deployed[color]
		r.stateMu.Unlock()
		if ok {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("%s never locked in", color)
}
