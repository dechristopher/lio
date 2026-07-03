package room

import (
	"fmt"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// vacancyGrace returns how long the room should wait, once it has gone empty,
// before it is torn down — given whether anyone has ever connected during its
// lifetime.
//
// An open challenge that has been occupied and then vacated (its creator's
// socket dropped) gets the short reconnect grace, so routine reconnection churn
// — the client's stale-socket watchdog, a throttled background tab, a brief
// network blip — never clears a live seek before the client reconnects. Every
// other vacancy keeps the full grace: nobody has arrived yet (initial page
// load), or an opponent has already joined and a game is committed mid-handoff
// (hasOpenSeat is already false, since Join fills the seat before the redirect).
func (r *Instance) vacancyGrace(connected bool) time.Duration {
	if connected && r.hasOpenSeat() {
		return reconnectGrace
	}
	return roomExpiryTime
}

// handle waiting for players period
// waits for <roomExpiryTime> seconds if all players are disconnected and will
// proceed to clean up the room if exceeded
func (r *Instance) handleWaitingForPlayers() {
	// single cleanup timer, armed only while nobody is connected/waiting.
	// Created already-armed for the initial grace period before anyone shows
	// up. We reuse it via arm/stop helpers (below) instead of allocating a new
	// timer on every state change, which previously leaked timer goroutines and
	// risked a stale fire from an abandoned timer.
	cleanupTimer := time.NewTimer(roomExpiryTime)
	defer cleanupTimer.Stop()

	// armCleanup (re)starts the timer for the given grace, draining any pending
	// fire first so a previous expiry can't trigger an immediate, spurious
	// cleanup. The duration differs by why the room is empty (see
	// reconcileCleanup): the full initial grace before anyone has connected vs.
	// the shorter reconnect grace once the creator has been present and dropped.
	armCleanup := func(grace time.Duration) {
		if !cleanupTimer.Stop() {
			select {
			case <-cleanupTimer.C:
			default:
			}
		}
		cleanupTimer.Reset(grace)
	}
	// stopCleanup disarms the timer, draining any pending fire.
	stopCleanup := func() {
		if !cleanupTimer.Stop() {
			select {
			case <-cleanupTimer.C:
			default:
			}
		}
	}

	waitingRoom := channel.Map.GetSockMap(fmt.Sprintf("%s%s", waiting, r.ID))
	waitingListener := waitingRoom.Listen()
	defer waitingRoom.UnListen(waitingListener)

	gameRoom := channel.Map.GetSockMap(r.ID)
	connectionListener := gameRoom.Listen()
	defer gameRoom.UnListen(connectionListener)

	// occupants is how many people with a stake in the room are present right
	// now: everyone on the challenge (waiting) page plus every *seated* player
	// connected on the game channel. Seated presence — not the game channel's
	// raw uid count — matters on the game side: once both seats are claimed a
	// stranger's socket is a spectator, and a lurking spectator must not keep
	// an abandoned challenge alive.
	occupants := func() int {
		return waitingRoom.Length() + r.connectedSeats()
	}

	// connected records whether anyone has ever connected to this room. Until
	// the creator actually shows up we keep the full grace period (they need
	// time to load the challenge page and open the socket); only once someone
	// has connected does the room going empty trigger the instant teardown.
	var connected bool

	// reconcileCleanup adjusts the cleanup timer for the current occupancy. It no
	// longer tears the room down itself — every vacancy arms the timer and the
	// teardown happens on the timer fire — so it always returns false; the return
	// is kept so callers read uniformly with the timer-fire path.
	//
	// The grace differs by why the room is empty:
	//
	//   - An open challenge the creator has connected to and then vacated gets the
	//     shorter reconnect grace. Previously this case tore the room down the
	//     instant the socket count hit zero, which meant routine reconnection
	//     churn — the stale-socket watchdog, a throttled background tab, a network
	//     blip — killed a live challenge before the client's reconnect landed. The
	//     grace timer survives those transient drops and is cancelled the moment
	//     the creator reconnects (occupants() > 0 below), so a challenge waits
	//     indefinitely while its creator is present and only clears once they are
	//     genuinely gone for reconnectGrace.
	//
	//   - Every other vacancy keeps the full grace: the creator may not have
	//     arrived yet (initial load), or an opponent has already joined and we are
	//     mid-handoff from the challenge page to the board (a brief window where
	//     both channels can read empty even though the game is committed —
	//     hasOpenSeat is already false by then because Join fills the seat before
	//     the redirect, so it is correctly excluded from the reconnect grace).
	reconcileCleanup := func() {
		if occupants() > 0 {
			connected = true
			util.DebugFlag("room", str.CRoom, "[%s] stopped cleanup timer, players connected", r.ID)
			stopCleanup()
			return
		}

		grace := r.vacancyGrace(connected)
		util.DebugFlag("room", str.CRoom, "[%s] room empty, teardown in %s unless someone (re)connects", r.ID, grace)
		armCleanup(grace)
	}

	util.DebugFlag("room", str.CRoom, "[%s] waiting for players", r.ID)

	for {
		select {
		case <-waitingListener:
			reconcileCleanup()
		case <-connectionListener:
			reconcileCleanup()

			// nothing more to do while the room is empty (timer already armed)
			if occupants() == 0 {
				continue
			}

			// automatically ready bot players; otherwise both *seated* players
			// must be connected on the game channel before the game can start.
			// Seat-keyed presence (bothPlayersConnected) rather than a raw
			// distinct-uid count, so a spectator connecting during the join
			// handoff (seats claimed, joiner's socket not yet open) can never
			// trigger a premature start.
			if r.HasBot() || r.bothPlayersConnected() {
				util.DebugFlag("room", str.CRoom, "[%s] players connected, game ready", r.ID)
				if err := r.event(EventPlayersConnected); err != nil {
					panic(err)
				}
				return
			}
		case <-cleanupTimer.C:
			// Re-check occupancy at fire time. A (re)connect can race the timer
			// fire — both cases become ready and select may pick the timer even
			// though someone is now present (a creator who reconnected within the
			// grace, or a joiner who arrived) — so don't tear down a room that is
			// no longer empty. The pending listener event re-reconciles next loop.
			if occupants() > 0 {
				util.DebugFlag("room", str.CRoom, "[%s] cleanup fire raced a (re)connect, keeping room", r.ID)
				continue
			}
			r.abandoned = true
			// room expired, clean up
			util.DebugFlag("room", str.CRoom, "[%s] room expired, cleaning up", r.ID)
			err := r.event(EventPlayerAbandons)
			if err != nil {
				panic(err)
			}
			return
		case control := <-r.controlChannel:
			// if room cancelled, halt immediately. The flag is set here, in
			// the room routine goroutine, so the routine loop exits after this
			// handler returns without racing the caller of Cancel().
			if control.Type == message.Cancel {
				r.cancelled = true
				return
			}
		}
	}
}
