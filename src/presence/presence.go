// Package presence tracks site-wide "who's online right now" for visitors that
// hold no room connection. Browsers sitting on the home page open only the
// live-games TV stream (/socket/tv) — a read-only global channel that presence
// intentionally ignores (it is not a room and is not walked by HomeListing) —
// and otherwise poll /home/activity over HTTP every few seconds, so their
// presence is inferred from a recent request timestamp per user id.
//
// In-room presence (seated players and spectators) is already authoritative via
// the channel SockMaps, so this package only fills the home-page gap. Online
// unions the two sources by user id, so a single human is never double-counted
// whether they are polling the home page, sitting in a room, or both.
package presence

import (
	"sync"
	"time"
)

// ttl is how long after a user's last home-page request we still consider them
// present. It must comfortably exceed the home page's poll interval (5s) so a
// single dropped poll never blinks an otherwise-active viewer offline.
const ttl = 12 * time.Second

var (
	mu      sync.Mutex
	pollers = make(map[string]time.Time)
)

// Touch records that the given user id was just seen on the home page. Empty
// ids (a request before the user context cookie is established) are ignored.
func Touch(uid string) {
	if uid == "" {
		return
	}
	mu.Lock()
	pollers[uid] = time.Now()
	mu.Unlock()
}

// Online returns the number of distinct humans currently online: the supplied
// set of in-room user ids unioned with every home-page poller seen within the
// ttl window. Stale pollers are pruned as they are scanned, so reads double as
// the map's garbage collector and the home page (polled continuously by every
// active viewer) keeps it bounded without a separate sweeper.
func Online(inRoom map[string]struct{}) int {
	now := time.Now()

	mu.Lock()
	defer mu.Unlock()

	n := len(inRoom)
	for uid, seen := range pollers {
		if now.Sub(seen) > ttl {
			delete(pollers, uid)
			continue
		}
		if _, alsoInRoom := inRoom[uid]; !alsoInRoom {
			n++
		}
	}
	return n
}
