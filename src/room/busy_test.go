package room

import (
	"testing"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/player"
)

func timeNowPlusInterlude() time.Time { return time.Now().Add(matchInterludeWindow) }
func zeroTime() time.Time             { return time.Time{} }

// seatRoom builds a bare Instance with the given account ids seated, under a
// distinct room id. It does not start a room routine: these tests are about the
// index, not the state machine.
func seatRoom(id string, white, black *int64) *Instance {
	r := &Instance{ID: id, players: player.Players{}}
	r.players[octad.White] = &player.Player{ID: id + "w", UserID: white}
	r.players[octad.Black] = &player.Player{ID: id + "b", UserID: black}
	return r
}

// registered builds a room seated by uid, primes its state machine at the given
// state, puts it in the room registry and registers its seats — everything
// Engaged / EngagedSeat / Seeks read.
func registered(t *testing.T, id, state string, seats ...*player.Player) *Instance {
	t.Helper()

	r := newTestInstance(t, "", "")
	r.ID = id
	r.stateMachine = newStateMachineAt(State(state))
	r.players = player.Players{}
	for i, p := range seats {
		if i == 0 {
			r.players[octad.White] = p
		} else {
			r.players[octad.Black] = p
		}
	}

	rooms.Store(r.ID, r)
	r.stateMu.Lock()
	r.setBusySeats()
	r.stateMu.Unlock()

	t.Cleanup(func() {
		rooms.Delete(r.ID)
		r.clearBusySeats()
	})
	return r
}

func acct(id int64) *int64 { return &id }

// TestSeatIndexTracksSeats: the index is a second copy of what the rooms hold,
// so the thing worth testing is that a room can only ever remove what it added.
func TestSeatIndexTracksSeats(t *testing.T) {
	r := seatRoom("a", acct(11), nil)

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
	r.players[octad.Black] = &player.Player{ID: "ab", UserID: acct(12)}
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

// TestSeatIndexCountsRoomsSeparately: one account can hold a seat in two rooms
// at once — a game, and a challenge of their own still waiting. A flag would
// clear on the first teardown while the other seat is still held, which would
// advertise them as challengeable mid-game.
func TestSeatIndexCountsRoomsSeparately(t *testing.T) {
	const id int64 = 21
	game := seatRoom("g", acct(id), acct(22))
	waiting := seatRoom("w", acct(id), nil)

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

// TestSeatIndexHoldsOneAccountOnBothSeats: an unrated room may seat the same
// account twice (Join only refuses self-play when the room is rated), so the
// per-room entry has to be counted. A set would clear on the first seat's
// teardown while the second is still held.
func TestSeatIndexHoldsOneAccountOnBothSeats(t *testing.T) {
	const id int64 = 41
	r := &Instance{ID: "selfplay", players: player.Players{}}
	r.players[octad.White] = &player.Player{ID: "laptop", UserID: acct(id)}
	r.players[octad.Black] = &player.Player{ID: "phone", UserID: acct(id)}

	r.stateMu.Lock()
	r.setBusySeats()
	r.stateMu.Unlock()
	if !AccountBusy(id) {
		t.Fatal("account on both seats is not busy")
	}

	// one seat leaves; the account is still at the board on the other
	r.stateMu.Lock()
	r.players[octad.Black] = &player.ToJoin
	r.setBusySeats()
	r.stateMu.Unlock()
	if !AccountBusy(id) {
		t.Fatal("releasing one of two seats cleared the account")
	}

	r.clearBusySeats()
	if AccountBusy(id) {
		t.Fatal("account still busy after teardown")
	}
}

// TestSeatIndexAnonymousSeatsAreSessionsNotAccounts: an anonymous seat has no
// account to key on, but it is still a session that must be held to one game —
// anonymous play is most of the site's traffic, and an account-only index could
// not gate any of it. A bot is not a person and registers nothing at all.
func TestSeatIndexAnonymousSeatsAreSessionsNotAccounts(t *testing.T) {
	r := &Instance{ID: "anonroom", players: player.Players{}}
	r.players[octad.White] = &player.Player{ID: "anon"}
	r.players[octad.Black] = &player.Player{ID: "bot", IsBot: true, UserID: acct(31)}

	r.stateMu.Lock()
	r.setBusySeats()
	r.stateMu.Unlock()
	t.Cleanup(r.clearBusySeats)

	if AccountBusy(0) {
		t.Fatal("the anonymous marker reads as a busy account")
	}
	if AccountBusy(31) {
		t.Fatal("a bot seat registered its account id")
	}
	// exactly one seat: the anonymous human, keyed by session
	if len(r.busySeats) != 1 || r.busySeats[0].uid != "anon" {
		t.Fatalf("room registered %v, want just the anonymous session", r.busySeats)
	}
	if own, _ := heldRooms("anon", 0); len(own) != 1 || own[0] != "anonroom" {
		t.Fatalf("anonymous session holds %v, want [anonroom]", own)
	}
	if own, _ := heldRooms("bot", 31); len(own) != 0 {
		t.Fatalf("bot session holds %v, want none", own)
	}
}

// TestEngagedTracksTheMatchNotTheSeat is the predicate the whole one-game rule
// rests on. A seat outlives the match played in it, and blocking on the seat
// would lock a player out of a new game for the two minutes a finished bot
// room stays open for analysis.
func TestEngagedTracksTheMatchNotTheSeat(t *testing.T) {
	cases := []struct {
		state string
		want  bool
		why   string
	}{
		{string(StateWaitingForPlayers), false, "a seek nobody has taken is not a commitment"},
		{string(StateGameReady), true, "both players are at the board"},
		{string(StateDeploy), true, "the blind deploy is a phase of the game"},
		{string(StateGameOngoing), true, "moves are being played"},
		{string(StateGameOver), false, "a rematch is an offer, and analysis is a courtesy"},
		{string(StateRoomOver), false, "the room is reaping itself"},
	}

	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			r := registered(t, "eng-"+tc.state, tc.state,
				&player.Player{ID: "u1", UserID: acct(51)})

			if got := r.Engaged(); got != tc.want {
				t.Fatalf("Engaged() = %v, want %v: %s", got, tc.want, tc.why)
			}
			if got := Engaged("u1", 51); got != tc.want {
				t.Fatalf("Engaged(uid) = %v, want %v: %s", got, tc.want, tc.why)
			}
			// Busy is the wider question and is true throughout: the seat is
			// held in every one of these states
			if !AccountBusy(51) {
				t.Fatal("a seated account reads as not busy")
			}
		})
	}
}

// TestEngagedCoversTheRaceToInterlude: a race-to match pauses in StateGameOver
// between games and then starts the next one *by itself*. A player released
// during those seconds would be dropped into a second live board with no action
// of their own.
func TestEngagedCoversTheRaceToInterlude(t *testing.T) {
	r := registered(t, "interlude", string(StateGameOver),
		&player.Player{ID: "u2", UserID: acct(52)})

	if Engaged("u2", 52) {
		t.Fatal("a finished game holds its players")
	}

	r.stateMu.Lock()
	r.setNextGameDeadlineLocked(timeNowPlusInterlude())
	r.stateMu.Unlock()

	if !Engaged("u2", 52) {
		t.Fatal("the interlude released a player whose next game auto-starts")
	}

	// the next game begins: the deadline is void with it
	r.stateMu.Lock()
	r.setNextGameDeadlineLocked(zeroTime())
	r.stateMu.Unlock()

	if Engaged("u2", 52) {
		t.Fatal("clearing the interlude deadline left the flag set")
	}
}

// TestEngagedFoldsAcrossSessions: one account signed in on a laptop and a phone
// holds two uids and is one player. The phone must be refused a second game and
// must be able to find the laptop's.
func TestEngagedFoldsAcrossSessions(t *testing.T) {
	const id int64 = 61
	registered(t, "laptopgame", string(StateGameOngoing),
		&player.Player{ID: "laptop", UserID: acct(id)})

	if !Engaged("laptop", id) {
		t.Fatal("the session holding the seat is not engaged")
	}
	if !Engaged("phone", id) {
		t.Fatal("another session of the same account is not engaged")
	}
	if Engaged("phone", 0) {
		t.Fatal("an anonymous session inherited an account's game")
	}
	if Engaged("stranger", 62) {
		t.Fatal("an unrelated account is engaged")
	}
}

// TestEngagedSeatPrefersOwnSession: seats are keyed by uid, so the room this
// session holds itself is the one it can actually play. Offering the other
// device's game first would send somebody to a board they can only watch.
func TestEngagedSeatPrefersOwnSession(t *testing.T) {
	const id int64 = 71
	// alphabetically first, so a naive sort would pick it
	registered(t, "aaa-other", string(StateGameOngoing),
		&player.Player{ID: "phone", UserID: acct(id)})
	registered(t, "zzz-mine", string(StateGameOngoing),
		&player.Player{ID: "laptop", UserID: acct(id)})

	ref, ok := EngagedSeat("laptop", id)
	if !ok {
		t.Fatal("no engaged seat found")
	}
	if ref.RoomID != "zzz-mine" {
		t.Fatalf("EngagedSeat picked %q, want the session's own room", ref.RoomID)
	}
	if !ref.OwnSession {
		t.Fatal("the session's own seat is not marked OwnSession")
	}

	// a session with no seat of its own is told about the account's game, and
	// told that it belongs to another device
	ref, ok = EngagedSeat("tablet", id)
	if !ok {
		t.Fatal("no engaged seat found for a second device")
	}
	if ref.OwnSession {
		t.Fatal("another device's seat was marked OwnSession")
	}
	if ref.Label == "" {
		t.Fatal("the reconnect bar has nothing to render")
	}
}

// TestSeeksListsOnlyWaitingRooms: creating a game supersedes the seeks a
// session is sitting on, and must not reach anything else it holds.
func TestSeeksListsOnlyWaitingRooms(t *testing.T) {
	registered(t, "myseek", string(StateWaitingForPlayers),
		&player.Player{ID: "u3", UserID: acct(81)})
	registered(t, "mygame", string(StateGameOngoing),
		&player.Player{ID: "u3", UserID: acct(81)})
	// another session of the same account: superseding from one device must not
	// cancel a challenge somebody is watching on another
	registered(t, "otherseek", string(StateWaitingForPlayers),
		&player.Player{ID: "u4", UserID: acct(81)})

	got := Seeks("u3")
	if len(got) != 1 || got[0] != "myseek" {
		t.Fatalf("Seeks = %v, want just the session's own waiting room", got)
	}
}
