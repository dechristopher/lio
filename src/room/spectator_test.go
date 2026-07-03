package room

import (
	"testing"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/player"
)

// TestCanJoin locks the player-vs-spectator routing decision for every room
// shape. The bot-room case is the regression that shipped spectators the
// interactive player page: a bot seat has no uid, and HasTwoPlayers used to
// read it as an open seat, so CanJoin offered the room to any third visitor.
func TestCanJoin(t *testing.T) {
	t.Run("full human room", func(t *testing.T) {
		r := newTestInstance(t, "wp", "bp")
		if asPlayer, _ := r.CanJoin("wp"); !asPlayer {
			t.Fatal("a seated player must re-enter as a player")
		}
		asPlayer, asSpectator := r.CanJoin("stranger")
		if asPlayer || !asSpectator {
			t.Fatalf("third visitor = (player %t, spectator %t), want spectator", asPlayer, asSpectator)
		}
	})

	t.Run("open human challenge", func(t *testing.T) {
		r := newTestInstance(t, "wp", "")
		if asPlayer, _ := r.CanJoin("stranger"); !asPlayer {
			t.Fatal("a visitor to an open challenge joins as a player")
		}
	})

	t.Run("bot room", func(t *testing.T) {
		r := newTestInstance(t, "wp", "")
		r.players[octad.Black] = &player.Player{IsBot: true}
		if asPlayer, _ := r.CanJoin("wp"); !asPlayer {
			t.Fatal("the human must re-enter their bot game as a player")
		}
		asPlayer, asSpectator := r.CanJoin("stranger")
		if asPlayer || !asSpectator {
			t.Fatalf("bot-room visitor = (player %t, spectator %t), want spectator", asPlayer, asSpectator)
		}
	})
}

// TestIsPlayer locks the socket-layer spectator gate: only seated uids are
// players; everyone else (including the empty uid) is a spectator.
func TestIsPlayer(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")

	if !r.IsPlayer("wp") || !r.IsPlayer("bp") {
		t.Fatal("seated players must not read as spectators")
	}
	if r.IsPlayer("stranger") {
		t.Fatal("a non-seated uid must read as a spectator")
	}
	if r.IsPlayer("") {
		t.Fatal("the empty uid must never read as a player")
	}
}

// TestPlayerIDs locks the seat snapshot the crowd broadcaster keys off.
func TestPlayerIDs(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	w, b := r.PlayerIDs()
	if w != "wp" || b != "bp" {
		t.Fatalf("PlayerIDs = (%q, %q), want (wp, bp)", w, b)
	}
}

// TestSpectatorPresenceExcluded is the waiting-state race fix in miniature: a
// spectator socket on the game channel must count toward neither the
// players-connected transition (bothPlayersConnected) nor seated occupancy
// (connectedSeats), while seated connections count normally.
func TestSpectatorPresenceExcluded(t *testing.T) {
	r := newTestInstance(t, "wp", "bp")
	r.ID = "spectest-presence" // unique game channel in the global map
	sm := channel.Map.GetSockMap(r.ID)
	defer sm.Cleanup()

	// one seated player + one spectator: not both connected, one seat occupied
	sm.Track(channel.NewSocket(nil, "wp", "c1", ""))
	sm.Track(channel.NewSocket(nil, "spectator", "c1", ""))
	if r.bothPlayersConnected() {
		t.Fatal("a spectator must not satisfy the second seat's presence")
	}
	if got := r.connectedSeats(); got != 1 {
		t.Fatalf("connectedSeats = %d, want 1 (spectator excluded)", got)
	}

	// second seat connects: now both are present regardless of the spectator
	sm.Track(channel.NewSocket(nil, "bp", "c1", ""))
	if !r.bothPlayersConnected() {
		t.Fatal("both seated players connected must read as both present")
	}
	if got := r.connectedSeats(); got != 2 {
		t.Fatalf("connectedSeats = %d, want 2", got)
	}

	// players leave, spectator lingers: seated occupancy must read empty so
	// the waiting-state cleanup timer can arm (a lurker can't pin the room)
	sm.UnTrack("wp", "c1")
	sm.UnTrack("bp", "c1")
	if got := r.connectedSeats(); got != 0 {
		t.Fatalf("connectedSeats = %d, want 0 with only a spectator left", got)
	}
}
