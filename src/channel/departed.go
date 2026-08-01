package channel

import (
	"sync"
	"time"
)

// Recently-departed sessions, so presence can still name somebody who was here
// a moment ago.
//
// Connected() answers "who holds a socket right now", which is the exact truth
// and is what a challenge has to be gated on. It is a poor answer to "who is
// around", though: it empties the moment a tab closes, and it drops somebody for
// the length of a page load every time they navigate, because every page holds
// its own socket. The home page's roster asks the second question, so it needs
// the memory this file keeps (arch/HOME_ACTIVITY_STREAMING.md).
//
// The record is a socket *close event*, not a timestamp inferred from polling.
// That distinction matters — see the note in package presence, which deleted an
// HTTP-poll TTL map to get here and must not be read as having regained one.

// Departure is one session that closed a socket, and the account it was
// authenticated as at the time. The zero Account means an anonymous visitor,
// exactly as it does on Socket and in Connected().
type Departure struct {
	Account Account
	At      time.Time
}

const (
	// departedRetention bounds how long a closed session is remembered at all.
	// It is the package's own upper bound rather than any caller's window: a
	// caller asking about the last 15 minutes must not evict a record another
	// caller with a longer window would still want, so Departed filters to the
	// window it was asked for and only this constant ever deletes.
	departedRetention = 30 * time.Minute
	// departedSoftCap is the size past which a write prunes before inserting.
	// Reads prune too, and on a live site they are frequent enough to keep the
	// map small on their own; this only bounds the case where nothing is asking
	// — nobody on the home page, no /system console open — while sessions keep
	// closing. It is a trigger to prune early, not a hard limit: a burst of
	// genuinely recent departures is allowed to exceed it.
	departedSoftCap = 4096
)

var departed struct {
	sync.Mutex
	at map[string]Departure
}

// recordDeparture remembers that a session closed a connection. It is called
// unconditionally by UnTrack, including for a uid that still holds other
// sockets: readers union this with Connected() and prefer the live side, so a
// stamp for somebody who is still here is harmless, and testing for it would
// mean a second directory walk under this lock.
func recordDeparture(uid string, acct Account) {
	if uid == "" {
		return
	}
	departed.Lock()
	defer departed.Unlock()
	if departed.at == nil {
		departed.at = make(map[string]Departure)
	}
	if len(departed.at) >= departedSoftCap {
		pruneDepartedLocked()
	}
	departed.at[uid] = Departure{Account: acct, At: time.Now()}
}

// clearDeparture forgets a session's departure because it came back. Track
// calls it, which is what makes a page navigation — close, then open — cost
// nothing: the stamp the close wrote is gone before any window could count it.
func clearDeparture(uid string) {
	if uid == "" {
		return
	}
	departed.Lock()
	defer departed.Unlock()
	delete(departed.at, uid)
}

// Departed returns the sessions that closed a socket within the given window,
// keyed by uid exactly as Connected() is — so a caller can fold the two together
// without reconciling two shapes, and can resolve a departed session against the
// same seat overlay it uses for a live one.
//
// A uid may appear here and in Connected() at once (another tab, another
// device). The live answer wins; see recordDeparture.
func Departed(within time.Duration) map[string]Departure {
	cutoff := time.Now().Add(-within)
	departed.Lock()
	defer departed.Unlock()
	pruneDepartedLocked()
	out := make(map[string]Departure, len(departed.at))
	for uid, d := range departed.at {
		if d.At.After(cutoff) {
			out[uid] = d
		}
	}
	return out
}

// ForgetDepartures drops every remembered departure.
//
// It exists for tests in other packages. This map is process-global and its
// entries outlive the SockMap that produced them by design, so one test's closed
// sockets would otherwise still read as recently active while the next test
// runs — the same isolation problem Cleanup solves for the live directory.
func ForgetDepartures() {
	departed.Lock()
	defer departed.Unlock()
	departed.at = nil
}

// pruneDepartedLocked drops everything past departedRetention. Callers hold the
// lock.
func pruneDepartedLocked() {
	cutoff := time.Now().Add(-departedRetention)
	for uid, d := range departed.at {
		if !d.At.After(cutoff) {
			delete(departed.at, uid)
		}
	}
}
