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

	hasWaitingPlayer := func() bool {
		return waitingRoom.Length() > 0
	}

	util.DebugFlag("room", str.CRoom, "[%s] waiting for players", r.ID)

	for {
		select {
		case waitingPlayers := <-waitingListener:
			// don't clean up the room if the challenger is actively waiting
			// for their opponent to accept the invite
			if waitingPlayers > 0 {
				util.DebugFlag("room", str.CRoom, "[%s] stopped cleanup timer, players waiting", r.ID)
				stopCleanup()
			} else {
				util.DebugFlag("room", str.CRoom, "[%s] no players waiting, cleanup timer enabled", r.ID)
				armCleanup()
			}
		case numPlayers := <-connectionListener:
			util.DebugFlag("room", str.CRoom, "[%s] room player count changed: %d", r.ID, numPlayers)
			// start cleanup timer if no players are connected
			if numPlayers == 0 && !hasWaitingPlayer() {
				util.DebugFlag("room", str.CRoom, "[%s] no players connected, cleanup timer enabled", r.ID)
				armCleanup()
				continue
			}

			// stop timer if one or more players are connected
			if numPlayers > 0 || hasWaitingPlayer() {
				util.DebugFlag("room", str.CRoom, "[%s] stopped cleanup timer, players connected", r.ID)
				stopCleanup()
			}

			// automatically ready bot players
			if r.HasBot() {
				numPlayers = 2
			}

			// both players connected, transition to StateGameReady
			if numPlayers == 2 {
				util.DebugFlag("room", str.CRoom, "[%s] players connected, game ready", r.ID)
				err := r.event(EventPlayersConnected)
				if err != nil {
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
