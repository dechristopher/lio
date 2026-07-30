package room

import (
	"testing"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/player"
)

// seatRoom builds a bare Instance with the given account ids seated. It does not
// start a room routine: these tests are about the index, not the state machine.
func seatRoom(white, black *int64) *Instance {
	r := &Instance{players: player.Players{}}
	r.players[octad.White] = &player.Player{ID: "w", UserID: white}
	r.players[octad.Black] = &player.Player{ID: "b", UserID: black}
	return r
}

func acct(id int64) *int64 { return &id }

// TestBusyIndexTracksSeats: the index is a second copy of what the rooms hold,
// so the thing worth testing is that a room can only ever remove what it added.
func TestBusyIndexTracksSeats(t *testing.T) {
	r := seatRoom(acct(11), nil)

	r.stateMu.Lock()
	r.setBusySeats()
	r.stateMu.Unlock()
	if !AccountBusy(11) {
		t.Fatal("creator is not busy after claiming a seat")
	}
	if AccountBusy(12) {
		t.Fatal("an unseated account reads as busy")
	}

	// the open seat is claimed: the joiner becomes busy, the creator stays busy
	r.stateMu.Lock()
	r.players[octad.Black] = &player.Player{ID: "b", UserID: acct(12)}
	r.setBusySeats()
	r.stateMu.Unlock()
	if !AccountBusy(11) || !AccountBusy(12) {
		t.Fatal("both seated accounts should be busy")
	}

	// reconciling again with no change must not double count, or teardown would
	// leave them busy forever
	r.stateMu.Lock()
	r.setBusySeats()
	r.stateMu.Unlock()

	r.clearBusySeats()
	if AccountBusy(11) || AccountBusy(12) {
		t.Fatal("teardown left accounts busy")
	}
}

// TestBusyIndexCountsRoomsSeparately: one account can hold a seat in two rooms
// at once — a game, and a challenge of their own still waiting. A flag would
// clear on the first teardown while the other seat is still held, which would
// advertise them as challengeable mid-game.
func TestBusyIndexCountsRoomsSeparately(t *testing.T) {
	const id int64 = 21
	game := seatRoom(acct(id), acct(22))
	waiting := seatRoom(acct(id), nil)

	for _, r := range []*Instance{game, waiting} {
		r.stateMu.Lock()
		r.setBusySeats()
		r.stateMu.Unlock()
	}
	if !AccountBusy(id) {
		t.Fatal("account seated in two rooms is not busy")
	}

	waiting.clearBusySeats()
	if !AccountBusy(id) {
		t.Fatal("one room closing cleared an account still seated in another")
	}

	game.clearBusySeats()
	if AccountBusy(id) {
		t.Fatal("account still busy after every room closed")
	}
	if AccountBusy(22) {
		t.Fatal("opponent still busy after the game closed")
	}
}

// TestBusyIndexIgnoresAnonymousAndBots: an anonymous seat has no account to key
// on, and a bot seat is not a person who can be challenged. Neither may leave a
// residue in the index — a stray 0 key would make AccountBusy(0) meaningful.
func TestBusyIndexIgnoresAnonymousAndBots(t *testing.T) {
	r := &Instance{players: player.Players{}}
	r.players[octad.White] = &player.Player{ID: "anon"}
	r.players[octad.Black] = &player.Player{ID: "bot", IsBot: true, UserID: acct(31)}

	r.stateMu.Lock()
	r.setBusySeats()
	r.stateMu.Unlock()

	if AccountBusy(0) {
		t.Fatal("the anonymous marker reads as a busy account")
	}
	if AccountBusy(31) {
		t.Fatal("a bot seat registered its account id")
	}
	if len(r.busySeats) != 0 {
		t.Fatalf("room registered %d seats, want 0", len(r.busySeats))
	}
	r.clearBusySeats()
}
