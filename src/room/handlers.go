package room

import "time"

const roomExpiryTime = time.Minute * 5
const abandonTimeout = time.Second * 20

// casualAbandonTimeout replaces abandonTimeout for untimed casual games
// against the computer (Params.Casual + a bot seat). A casual game never ends
// on the clock and its idle and first-move timeouts are disabled while the
// player is connected, so the disconnect timer is the only thing standing
// between an abandoned session and a room that lives forever — it is kept
// short to prevent room spam. Deliberately tighter than the client's
// reconnect churn (see reconnectGrace below): a transient drop costs only a
// solo bot game, and the player can spin up a fresh one instantly.
// Human-vs-human casual games keep the normal abandonTimeout instead — a real
// opponent is waiting, so a transient drop must survive like any human game.
const casualAbandonTimeout = time.Second * 5

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

// casualDisconnectTimeout returns how long this room tolerates a disconnected
// seat before abandoning, given its casual/bot configuration: the short
// casualAbandonTimeout for a casual bot game (the sole human left, nobody is
// waiting), the standard abandonTimeout otherwise. Timed games use it too —
// it simply resolves to the standard timeout for them.
func (r *Instance) casualDisconnectTimeout() time.Duration {
	if r.params.Casual && r.HasBot() {
		return casualAbandonTimeout
	}
	return abandonTimeout
}
