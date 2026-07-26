package room

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// Moderation's reach into live rooms (arch/ADMIN_MODERATION.md). A ban is
// immediate: the sanctioned account does not get to finish the game it was
// sanctioned for.

// CloseBanned is the WebSocket close code for a connection dropped because its
// account was banned. Distinct from 4001 (no identity) and 1012 (restart) so a
// client can tell "you were removed" from "reconnect shortly" — the client's
// generic reconnect then fails to re-establish anything, since the session
// rows are gone.
const CloseBanned = 4002

// ForfeitUser ends every live involvement of a banned account and closes its
// open sockets. Returns the number of rooms acted on (for the caller's log).
//
// Closing the sockets is not a nicety — it is half the sanction. Revoking the
// account's session rows stops new requests, but an in-flight WebSocket
// authenticated once at upgrade time and is keyed by session uid thereafter, so
// without this a banned player keeps playing the game they are sitting in.
//
// What "ending the room" means depends on the state, and follows what the room
// already does when a player simply walks away at that point:
//
//   - game_ongoing → resign their seat. This reuses the ordinary resignation
//     path rather than inventing a "forfeited" terminal state, so the game
//     archives like any other resignation, rates normally, and the opponent
//     gets a real win — no special case reaches the archive, the PGN or the
//     rating math. Waiting for their clock to flag instead would make the
//     opponent sit through it.
//   - waiting_for_players / deploy → cancel. No move has been played, so there
//     is no result to award; both states already treat a Cancel control as a
//     teardown.
//   - game_ready → nothing beyond the socket close. This state has no control
//     handler (it selects on moves, presence and its own timer), and the close
//     is enough: a timed game's first-move timer expires it on the existing
//     one-minute box, and a casual game re-arms its presence-driven timer the
//     moment the seat drops. Both are exactly the abandon path a player who
//     walked away before moving already takes.
//   - game_over / room_over → nothing; the room is already reaping itself.
//
// Every mutation goes through the room's control channel, never its state:
// room state belongs to the room's own goroutine, and reaching into it from a
// moderation handler would race the game it is trying to end.
//
// Safe to call for an account with no live rooms (the common case) — a walk of
// a map that is normally near-empty.
func ForfeitUser(userID int64) int {
	acted := 0
	rooms.Range(func(_, v interface{}) bool {
		r, ok := v.(*Instance)
		if !ok {
			return true
		}
		if r.forfeitSeat(userID) {
			acted++
		}
		return true
	})
	if acted > 0 {
		util.Info(str.CRoom, "moderation: ended %d live room(s) for banned user %d",
			acted, userID)
	}
	return acted
}

// forfeitSeat ends this room for the given account if it holds a seat here.
// Reports whether anything was done.
func (r *Instance) forfeitSeat(userID int64) bool {
	// find the seat's session uid: the control path identifies players by uid
	// (resignControl looks the color up with players.Lookup), while moderation
	// knows only the account id
	r.stateMu.Lock()
	seatUID := ""
	for _, p := range r.players {
		if p != nil && p.UserID != nil && *p.UserID == userID {
			seatUID = p.ID
			break
		}
	}
	r.stateMu.Unlock()

	if seatUID == "" {
		return false
	}

	// drop their live connections first, in every state: whatever the room does
	// next, the banned account must not be able to act in it again
	channel.CloseForUID(seatUID, CloseBanned, "account closed")

	state := r.State()
	switch state {
	case StateWaitingForPlayers:
		// an open challenge with nobody to wrong: tear it down rather than
		// leave a banned account's invitation live on the home page
		r.Cancel()

	case StateDeploy:
		// mid-deploy, before any move exists. The deploy loop treats a Cancel
		// as a teardown, which is the right outcome for a game that never began.
		r.sendControl(message.RoomControl{
			Player: seatUID,
			Type:   message.Cancel,
			Ctx:    channel.SocketContext{Channel: r.ID, UID: seatUID, MT: 1},
		})

	case StateGameOngoing:
		r.sendControl(message.RoomControl{
			Player: seatUID,
			Type:   message.Resign,
			Ctx:    channel.SocketContext{Channel: r.ID, UID: seatUID, MT: 1},
		})

	default:
		// game_ready: the socket close above is the whole action — see the
		// function comment. game_over / room_over: already reaping.
	}

	util.DebugFlag("room", str.CRoom, "[%s] moderation: banned user %d seat %s in state %s",
		r.ID, userID, seatUID, state)
	return true
}

// SeatedUsers reports the account ids currently seated in live rooms. Used by
// the moderation tooling to tell whether a sanction will interrupt a game
// before it is applied.
func SeatedUsers() map[int64]struct{} {
	seated := make(map[int64]struct{})
	rooms.Range(func(_, v interface{}) bool {
		r, ok := v.(*Instance)
		if !ok {
			return true
		}
		r.stateMu.Lock()
		for _, p := range r.players {
			if p != nil && p.UserID != nil {
				seated[*p.UserID] = struct{}{}
			}
		}
		r.stateMu.Unlock()
		return true
	})
	return seated
}

// CloseRoom tears down one room by id, for the operator's force-close on
// /system. Reports whether a room was found and told to close.
//
// It reuses the Cancel control the waiting state already handles, and the
// resign path for a game in progress — the same discipline ForfeitUser follows,
// and for the same reason: room state belongs to the room's own goroutine, so
// everything goes through the control channel rather than reaching in.
//
// A game in progress is *abandoned*, not decided: there is no honest result to
// award when an operator ends a game for operational reasons, and inventing one
// would put a fabricated outcome in two players' archives. This is the tool for
// clearing something stuck, not for settling a game.
func CloseRoom(roomID string) bool {
	v, ok := rooms.Load(roomID)
	if !ok {
		return false
	}
	r, ok := v.(*Instance)
	if !ok {
		return false
	}

	state := r.State()
	switch state {
	case StateWaitingForPlayers:
		r.Cancel()
	case StateGameReady, StateDeploy, StateGameOngoing:
		// Every one of these states consumes a Cancel and exits, so the room is
		// gone as soon as its routine picks the control up. Dropping the sockets
		// as well is what stops a client reconnecting into a room that is on its
		// way out — without it the close looked like it had done nothing.
		r.sendControl(message.RoomControl{
			Type: message.Cancel,
			Ctx:  channel.SocketContext{Channel: roomID, MT: 1},
		})
		channel.CloseAllOn(roomID, CloseRoomClosed, "room closed by a moderator")
	default:
		return false
	}

	util.Info(str.CRoom, "[%s] room closed by moderator (state %s)", roomID, state)
	return true
}

// CloseRoomClosed is the WebSocket close code for a connection dropped because
// a moderator closed the room. Distinct from 4002 (account banned) and 1012
// (restart) so the client can tell "this room is over" from "you were removed"
// and from "reconnect shortly".
const CloseRoomClosed = 4003
