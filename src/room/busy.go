package room

import "sync"

// Who is currently seated somewhere, answered in constant time.
//
// This backs the challenge controls (arch/NOTIFICATIONS.md Phase 2): the home
// roster asks it for every name it renders, and a player page asks it on every
// render, so it must not walk the room registry. A scan was the first version
// and would have grown with the number of live games — the one thing this
// lookup must not do.
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
// A room whose seats are anonymous contributes nothing: there is no account to
// key on, and an anonymous visitor cannot be challenged anyway.
var busyIndex = struct {
	mu sync.RWMutex
	// n counts seats per account rather than storing a bool. One account can be
	// seated in two rooms at once (a game and a challenge of their own waiting),
	// and a plain flag would clear on the first teardown while the other seat is
	// still held.
	n map[int64]int
}{n: make(map[int64]int)}

// AccountBusy reports whether the given account holds a seat in any room right
// now — playing, or waiting for an opponent in a challenge of its own.
//
// Deliberately wider than "playing": somebody sitting on their own waiting page
// has not started a game yet, but they are committed to the next one, so
// offering to challenge them would produce an invitation that sits unanswered.
func AccountBusy(userID int64) bool {
	if userID == 0 {
		return false
	}
	busyIndex.mu.RLock()
	defer busyIndex.mu.RUnlock()
	return busyIndex.n[userID] > 0
}

// setBusySeats reconciles this room's contribution to the index against the
// accounts it currently seats.
//
// The caller must hold stateMu, because the seat set is read from r.players.
// That is also why this does not call SeatUserIDs, which takes the same lock.
func (r *Instance) setBusySeats() {
	next := make([]int64, 0, 2)
	for _, p := range r.players {
		if p == nil || p.IsBot || p.UserID == nil {
			continue
		}
		// A seat can only be held by one account, but guard against listing the
		// same id twice: it would double-count, and the room would then have to
		// remove it twice to clear.
		dup := false
		for _, id := range next {
			if id == *p.UserID {
				dup = true
				break
			}
		}
		if !dup {
			next = append(next, *p.UserID)
		}
	}

	busyIndex.mu.Lock()
	defer busyIndex.mu.Unlock()

	for _, id := range r.busySeats {
		if !containsID(next, id) {
			if busyIndex.n[id] <= 1 {
				delete(busyIndex.n, id)
			} else {
				busyIndex.n[id]--
			}
		}
	}
	for _, id := range next {
		if !containsID(r.busySeats, id) {
			busyIndex.n[id]++
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

	busyIndex.mu.Lock()
	defer busyIndex.mu.Unlock()

	for _, id := range r.busySeats {
		if busyIndex.n[id] <= 1 {
			delete(busyIndex.n, id)
		} else {
			busyIndex.n[id]--
		}
	}
	r.busySeats = nil
}

func containsID(ids []int64, want int64) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
