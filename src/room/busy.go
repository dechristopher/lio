package room

import (
	"sort"
	"sync"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/www/ws/proto"
)

// Who is seated where, answered in constant time.
//
// This backs three things (arch/ONE_GAME_AT_A_TIME.md): the challenge controls
// (arch/NOTIFICATIONS.md Phase 2), the one-game-at-a-time rule, and the
// reconnect bar every page renders. The home roster asks it for every name it
// renders and a page render asks it for the viewer, so it must not walk the
// room registry. A scan was the first version and would have grown with the
// number of live games — the one thing this lookup must not do.
//
// It is an index, which means it is a second copy of a truth the rooms already
// hold, and a second copy can drift. Two things keep it honest:
//
//   - Each room reconciles its *own* contribution through setBusySeats, which
//     diffs against what that room last registered. Calling it twice with the
//     same seats is a no-op, and a room can only ever remove what it added.
//   - There are few places a room's seats change, and all of them are covered:
//     creation, a joiner claiming the open seat, rehydration after a restart,
//     and teardown. Rematches flip colors between the same two accounts, which
//     is not a membership change.
//
// # Two questions, not one
//
// The index answers two questions that must never be conflated:
//
//	Busy      holds a seat anywhere — playing, or waiting in a challenge of
//	          their own. Gates the challenge affordances: somebody sitting on
//	          their own waiting page has not started a game yet, but they are
//	          committed to the next one, so offering to challenge them would
//	          produce an invitation that sits unanswered.
//	Engaged   holds a seat in a room whose *match is still going*. Gates the
//	          one-game rule and drives the reconnect bar.
//
// Engaged is strictly narrower, and the difference is the whole reason the rule
// is usable. A seat outlives the match played in it: a finished bot game keeps
// its room for botAnalysisWindow so the player can review it, and a finished
// human game holds a rematch window. Blocking on a held seat would lock a
// player out of a new game for two minutes after every bot game they finish.
//
// # Session first, account second
//
// The uid is the primary key, not the account. Seats, sockets and turn checks
// are all keyed by uid, and it is the only identity an anonymous player has —
// an account-keyed index could not enforce anything against most of the site's
// traffic. The account is a fold over the uids, which is what makes one person
// signed in on a laptop and a phone read as one player.
//
// A bot holds no uid and is never indexed. Neither is the empty seat of a room
// still waiting for an opponent.

// seat is one room's contribution for one player: the session holding the seat
// and the account behind it (0 for an anonymous seat).
type seat struct {
	uid    string
	userID int64
}

// seatIndex maps each identity to the rooms it holds a seat in.
//
// The inner value is a count rather than a set membership because one account
// can legitimately hold both seats of the same room: Join only refuses an
// account playing itself in a *rated* room, so two sessions of one account can
// sit across an unrated board. A plain set would then clear on the first seat's
// teardown while the second is still held.
var seatIndex = struct {
	mu        sync.RWMutex
	byUID     map[string]map[string]int
	byAccount map[int64]map[string]int
}{
	byUID:     make(map[string]map[string]int),
	byAccount: make(map[int64]map[string]int),
}

// SeatRef names the room a session or account is committed to, in the form the
// reconnect bar renders.
type SeatRef struct {
	// RoomID is where to send them back to.
	RoomID string
	// Label describes the game in one line ("½ + 1 blitz vs Queen").
	Label string
	// OwnSession reports that the asking uid holds the seat itself, rather than
	// another session of the same account. It is the difference between
	// "reconnect" and "you are playing on another device": seats are keyed by
	// uid, so a different session of the same account opening that room is a
	// spectator of its own game until seat adoption exists.
	OwnSession bool
}

// idxAdd registers one room against one identity.
func idxAdd[K comparable](m map[K]map[string]int, key K, roomID string) {
	rooms, ok := m[key]
	if !ok {
		rooms = make(map[string]int, 1)
		m[key] = rooms
	}
	rooms[roomID]++
}

// idxDrop releases one room from one identity, forgetting the identity once it
// holds nothing. An identity with no rooms must not linger: AccountBusy would
// still answer from a present-but-empty map.
func idxDrop[K comparable](m map[K]map[string]int, key K, roomID string) {
	rooms, ok := m[key]
	if !ok {
		return
	}
	if rooms[roomID] <= 1 {
		delete(rooms, roomID)
	} else {
		rooms[roomID]--
	}
	if len(rooms) == 0 {
		delete(m, key)
	}
}

// idxRooms copies out the room ids an identity holds, sorted so a caller that
// has to pick one picks the same one every time.
//
// It copies, and the caller resolves room state *after* the lock is released.
// Holding this lock while taking a room's own locks would put two lock orders
// in play — see Engaged.
func idxRooms[K comparable](m map[K]map[string]int, key K) []string {
	rooms := m[key]
	if len(rooms) == 0 {
		return nil
	}
	out := make([]string, 0, len(rooms))
	for id := range rooms {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// AccountBusy reports whether the given account holds a seat in any room right
// now — playing, or waiting for an opponent in a challenge of their own.
//
// Deliberately wider than "playing": somebody sitting on their own waiting page
// has not started a game yet, but they are committed to the next one, so
// offering to challenge them would produce an invitation that sits unanswered.
func AccountBusy(userID int64) bool {
	if userID == 0 {
		return false
	}
	seatIndex.mu.RLock()
	defer seatIndex.mu.RUnlock()
	return len(seatIndex.byAccount[userID]) > 0
}

// heldRooms returns every room this session — and, folded in, this account —
// holds a seat in. Own-session rooms come first, so a caller picking one picks
// the seat this session can actually play.
func heldRooms(uid string, acctID int64) (own, other []string) {
	seatIndex.mu.RLock()
	defer seatIndex.mu.RUnlock()

	own = idxRooms(seatIndex.byUID, uid)
	if acctID == 0 {
		return own, nil
	}
	for _, id := range idxRooms(seatIndex.byAccount, acctID) {
		if !containsID(own, id) {
			other = append(other, id)
		}
	}
	return own, other
}

// Engaged reports whether this session, or any other session of the same
// account, is committed to a game in progress.
//
// This is the one-game-at-a-time gate. It is deliberately cheap and
// lock-conservative: it resolves each candidate room through Instance.Engaged,
// which reads the state machine and one atomic and takes no room's stateMu. A
// gate that took stateMu could be called from a handler already holding another
// room's stateMu (Join does exactly that), and two rooms gating each other
// simultaneously would deadlock.
//
// acctID may be 0 for an anonymous session, which is then gated on its uid
// alone — the only identity it has.
func Engaged(uid string, acctID int64) bool {
	return EngagedElsewhere(uid, acctID, "")
}

// EngagedElsewhere is Engaged asked from inside a room, ignoring that room.
//
// The rematch and next-game paths need it for opposite reasons. A finished game
// is not Engaged, so a plain Engaged answers correctly there — but a race-to
// interlude *is*, by way of the room asking the question, and gating on that
// would refuse every player their own next game.
func EngagedElsewhere(uid string, acctID int64, exceptRoomID string) bool {
	own, other := heldRooms(uid, acctID)
	for _, id := range append(own, other...) {
		if id == exceptRoomID {
			continue
		}
		if r, err := Get(id); err == nil && r.Engaged() {
			return true
		}
	}
	return false
}

// EngagedSeat returns the game this session or account is committed to, for the
// reconnect bar.
//
// Unlike Engaged it builds a label, which reads the room's seats and variant
// under that room's stateMu. Callers must therefore hold no room lock: it is
// for the render path and the socket push, never for a gate inside a room.
//
// A room this session holds itself wins over one held by another session of the
// same account, so a player at two devices is offered the seat they can play.
func EngagedSeat(uid string, acctID int64) (SeatRef, bool) {
	own, other := heldRooms(uid, acctID)

	for _, id := range own {
		if r, err := Get(id); err == nil && r.Engaged() {
			return SeatRef{
				RoomID:     id,
				Label:      r.seatLabel(uid, acctID),
				OwnSession: true,
			}, true
		}
	}
	for _, id := range other {
		if r, err := Get(id); err == nil && r.Engaged() {
			return SeatRef{RoomID: id, Label: r.seatLabel(uid, acctID)}, true
		}
	}
	return SeatRef{}, false
}

// Seeks returns the rooms this session is waiting in with nobody having joined
// — its own unaccepted challenges.
//
// Creating a new game supersedes these rather than being refused by them
// (arch/ONE_GAME_AT_A_TIME.md): a seek is a reservation, not a commitment, and
// refusing would name a room the player cannot see from wherever they are
// standing. Keyed on the session, not the account: superseding somebody's seek
// from another device would cancel a challenge they may still be watching.
func Seeks(uid string) []string {
	own, _ := heldRooms(uid, 0)
	var out []string
	for _, id := range own {
		if r, err := Get(id); err == nil && r.State() == StateWaitingForPlayers {
			out = append(out, id)
		}
	}
	return out
}

// Engaged reports whether this room still holds its players to a match in
// progress — the predicate behind the one-game rule.
//
// Live() is most of it. The exception is a race-to match's interlude, which
// sits in StateGameOver and then starts the next game *by itself*
// (handleMatchInterlude): a player who slipped away during those few seconds
// would be dropped into a second live board with no action of their own.
//
// Everything else in StateGameOver is genuinely finished — a human rematch is
// an offer, and a bot room's analysis window is a courtesy — so it releases the
// seat immediately rather than holding the player for the two minutes that room
// stays open.
//
// It takes no stateMu. State() reads the machine's own lock and inInterlude is
// an atomic, which is what lets a gate call this while holding another room's
// stateMu without introducing a lock cycle.
func (r *Instance) Engaged() bool {
	state := r.State()
	if state.Live() {
		return true
	}
	return state == StateGameOver && r.inInterlude.Load()
}

// seatLabel describes this room in one line for the reconnect bar, from the
// point of view of the player being told about it: the time control and the
// opponent they left sitting there.
//
// The opponent is resolved against the asking identity so the bar never names
// the reader back to themselves. A seat with neither uid nor account match is
// treated as the opponent, which is the right answer for a spectator-shaped
// caller and harmless for anyone else.
func (r *Instance) seatLabel(uid string, acctID int64) string {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	label := r.game.Variant.Name
	if !r.game.Variant.Casual {
		label += " " + r.game.Variant.SpeedGroup().String()
	}

	for _, color := range []octad.Color{octad.White, octad.Black} {
		p := r.players[color]
		if p == nil {
			continue
		}
		// A bot seat is checked before anything keyed on the session, because it
		// holds no uid at all: the engine's Player is a copy of player.ToJoin
		// with IsBot set, so its ID is empty — the same empty ID a seat nobody
		// has taken carries. Testing the id first read the bot as an open seat
		// and left the bar saying "½ + 1 blitz" with no opponent.
		if !p.IsBot {
			if p.ID == "" {
				continue // a seat still waiting for somebody
			}
			if p.ID == uid || (acctID != 0 && p.UserID != nil && *p.UserID == acctID) {
				continue // this is the reader's own seat
			}
		}
		return label + " vs " + r.seatNameLocked(p)
	}
	return label
}

// seatNameLocked names one seat for the bar: the bot's difficulty persona, the
// account username, or "Anonymous". The caller must hold stateMu.
func (r *Instance) seatNameLocked(p *player.Player) string {
	if p.IsBot {
		return r.botPersona().Name
	}
	if p.Username != "" {
		return p.Username
	}
	return "Anonymous"
}

// setBusySeats reconciles this room's contribution to the index against the
// players it currently seats.
//
// The caller must hold stateMu, because the seat set is read from r.players.
// That is also why this does not call SeatUserIDs, which takes the same lock.
func (r *Instance) setBusySeats() {
	next := make([]seat, 0, 2)
	for _, p := range r.players {
		// bots hold no session, and the placeholder for a seat nobody has taken
		// yet (player.ToJoin) carries an empty id
		if p == nil || p.IsBot || p.ID == "" {
			continue
		}
		// A seat can only be held by one session, but guard against listing the
		// same one twice: it would double-count, and the room would then have to
		// remove it twice to clear.
		s := seat{uid: p.ID}
		if p.UserID != nil {
			s.userID = *p.UserID
		}
		if !containsSeat(next, s) {
			next = append(next, s)
		}
	}

	seatIndex.mu.Lock()
	defer seatIndex.mu.Unlock()

	for _, s := range r.busySeats {
		if !containsSeat(next, s) {
			r.dropSeatLocked(s)
		}
	}
	for _, s := range next {
		if !containsSeat(r.busySeats, s) {
			r.addSeatLocked(s)
		}
	}
	r.busySeats = next
}

// clearBusySeats drops everything this room registered. It runs at teardown,
// which is the only path that must release seats the room still holds.
//
// It takes stateMu itself: cleanup runs from the room routine's deferred call,
// outside any lock the seat writers hold.
func (r *Instance) clearBusySeats() {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	seatIndex.mu.Lock()
	defer seatIndex.mu.Unlock()

	for _, s := range r.busySeats {
		r.dropSeatLocked(s)
	}
	r.busySeats = nil
}

// addSeatLocked / dropSeatLocked register and release one seat under both keys.
// The caller must hold seatIndex.mu.
func (r *Instance) addSeatLocked(s seat) {
	idxAdd(seatIndex.byUID, s.uid, r.ID)
	if s.userID != 0 {
		idxAdd(seatIndex.byAccount, s.userID, r.ID)
	}
}

func (r *Instance) dropSeatLocked(s seat) {
	idxDrop(seatIndex.byUID, s.uid, r.ID)
	if s.userID != 0 {
		idxDrop(seatIndex.byAccount, s.userID, r.ID)
	}
}

// PushLiveGame re-sends the reconnect bar's contents to one session and to every
// other session of the same account (arch/ONE_GAME_AT_A_TIME.md).
//
// It always recomputes from the index rather than being told what to say. A room
// that just ended is not the authority on what this player's bar shows — they
// may hold a seat somewhere else — and a frame built from one room's point of
// view would clear a bar that should still be pointing at another game.
//
// The account fan-out costs one walk of the socket directory, at state
// boundaries only (a handful per game). It is what keeps a phone's bar honest
// while the game is played on a laptop; without it that device is correct only
// from its next page load.
//
// Callers must hold no room's stateMu: resolving the label takes the seated
// room's lock.
func PushLiveGame(uid string, acctID int64) {
	sendLiveGame(uid, acctID)
	if acctID == 0 {
		return
	}
	for otherUID, acct := range channel.Connected() {
		if acct.ID == acctID && otherUID != uid {
			sendLiveGame(otherUID, acctID)
		}
	}
}

// sendLiveGame writes one session's current bar state. A session with no game
// gets the zero payload, which is the clear — sent rather than inferred, since a
// bar left standing after a game ended would offer a trip to a room being torn
// down.
func sendLiveGame(uid string, acctID int64) {
	var p proto.LiveGamePayload
	if ref, ok := EngagedSeat(uid, acctID); ok {
		p = proto.LiveGamePayload{
			RoomID: ref.RoomID,
			Label:  ref.Label,
			Own:    ref.OwnSession,
		}
	}
	channel.SendToUID(uid, proto.LiveGameMessage(p))
}

// publishLiveGame tells this room's seats what their bar should now say. It runs
// at every state boundary, which is where a game becomes live and where it stops
// being live.
//
// seats is passed in rather than read here so teardown can publish *after*
// releasing its entries — the seats have to be gone from the index before the
// frame is computed, or the clear would still describe the room being torn down.
func publishLiveGame(seats []seat) {
	for _, s := range seats {
		PushLiveGame(s.uid, s.userID)
	}
}

// heldSeats copies this room's registered seats for a publish that must outlive
// the lock.
func (r *Instance) heldSeats() []seat {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	out := make([]seat, len(r.busySeats))
	copy(out, r.busySeats)
	return out
}

// seatAccountID is the account behind a seat, or 0 for an anonymous one and for
// a seat nobody holds — the shape the index and its predicates take.
func seatAccountID(p *player.Player) int64 {
	if p == nil || p.UserID == nil {
		return 0
	}
	return *p.UserID
}

func containsSeat(seats []seat, want seat) bool {
	for _, s := range seats {
		if s == want {
			return true
		}
	}
	return false
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
