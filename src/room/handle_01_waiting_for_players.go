package room

import (
	"fmt"
	"time"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

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

	// armCleanup (re)starts the timer, draining any pending fire first so a
	// previous expiry can't trigger an immediate, spurious cleanup.
	armCleanup := func() {
		if !cleanupTimer.Stop() {
			select {
			case <-cleanupTimer.C:
			default:
			}
		}
		cleanupTimer.Reset(roomExpiryTime)
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

	// occupants is the number of live connections across both the challenge
	// (waiting) and game channels — how many people are in the room right now,
	// regardless of which page they are on.
	occupants := func() int {
		return waitingRoom.Length() + gameRoom.Length()
	}

	// connected records whether anyone has ever connected to this room. Until
	// the creator actually shows up we keep the full grace period (they need
	// time to load the challenge page and open the socket); only once someone
	// has connected does the room going empty trigger the instant teardown.
	var connected bool

	// reconcileCleanup adjusts the cleanup timer for the current occupancy and
	// reports whether the room was torn down (so the caller exits the handler).
	//
	// When the last occupant leaves an open challenge that no opponent has
	// joined, the room is junk — its creator abandoned it before anyone joined
	// or any game began — so we tear it down immediately instead of letting it
	// linger out the grace period. Every other vacancy keeps the grace timer:
	// the creator may not have arrived yet, or an opponent has already joined and
	// we are mid-handoff from the challenge page to the board (a brief window
	// where both channels can read empty even though the game is committed —
	// hasOpenSeat is already false by then because Join fills the seat before the
	// redirect, so it is correctly excluded here).
	reconcileCleanup := func() (teardown bool) {
		if occupants() > 0 {
			connected = true
			util.DebugFlag("room", str.CRoom, "[%s] stopped cleanup timer, players connected", r.ID)
			stopCleanup()
			return false
		}

		if connected && r.hasOpenSeat() {
			util.DebugFlag("room", str.CRoom, "[%s] open challenge vacated before any game, cleaning up", r.ID)
			r.abandoned = true
			if err := r.event(EventPlayerAbandons); err != nil {
				panic(err)
			}
			return true
		}

		util.DebugFlag("room", str.CRoom, "[%s] no players connected, cleanup timer enabled", r.ID)
		armCleanup()
		return false
	}

	util.DebugFlag("room", str.CRoom, "[%s] waiting for players", r.ID)

	for {
		select {
		case <-waitingListener:
			if reconcileCleanup() {
				return
			}
		case <-connectionListener:
			if reconcileCleanup() {
				return
			}

			// nothing more to do while the room is empty (timer already armed)
			if occupants() == 0 {
				continue
			}

			// automatically ready bot players; otherwise both human seats must
			// be connected on the game channel before the game can start
			gamePlayers := gameRoom.Length()
			if r.HasBot() {
				gamePlayers = 2
			}

			// both players connected, transition to StateGameReady
			if gamePlayers == 2 {
				util.DebugFlag("room", str.CRoom, "[%s] players connected, game ready", r.ID)
				if err := r.event(EventPlayersConnected); err != nil {
					panic(err)
				}
				return
			}
		case <-cleanupTimer.C:
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
