package room

import "time"

const roomExpiryTime = time.Minute * 5
const abandonTimeout = time.Second * 20

// reconnectGrace is how long a waiting open challenge survives its creator going
// offline before it is torn down. Unlike roomExpiryTime (the initial grace given
// before anyone has ever connected), this is the *reconnect* window applied once
// the creator has been present and then their socket drops. It must comfortably
// outlast a transient disconnect — the client's stale-socket watchdog (~3 missed
// pongs ≈ 15s) plus its reconnect backoff (capped at ~30s), a backgrounded-tab
// throttle, or a brief network blip — so routine reconnection churn never clears
// a live challenge, while a genuinely abandoned seek still clears from the Open
// Challenges feed within a minute. Cancelled the instant the creator reconnects.
const reconnectGrace = time.Second * 60

// idleTimeout is how long a bot game will wait on a socket-connected human who
// has not made a single move this game before treating them as having abandoned
// it. It is more generous than abandonTimeout (which keys off an actual socket
// disconnect) because a real player may legitimately take time over their first
// reply; it only needs to be well under a typical clock so a no-show game is
// cleaned up promptly instead of being played out to a flag for nobody.
const idleTimeout = time.Second * 30
