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
	cleanupTimer := time.NewTimer(time.Minute * 15)
	defer cleanupTimer.Stop()

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
				cleanupTimer.Stop()
			} else {
				util.DebugFlag("room", str.CRoom, "[%s] no players waiting, cleanup timer enabled", r.ID)
				cleanupTimer = time.NewTimer(roomExpiryTime)
			}
		case numPlayers := <-connectionListener:
			util.DebugFlag("room", str.CRoom, "[%s] room player count changed: %d", r.ID, numPlayers)
			// start cleanup timer if no players are connected
			if numPlayers == 0 && !hasWaitingPlayer() {
				util.DebugFlag("room", str.CRoom, "[%s] no players connected, cleanup timer enabled", r.ID)
				cleanupTimer = time.NewTimer(roomExpiryTime)
				continue
			}

			// stop timer if one or more players are connected
			if numPlayers > 0 || hasWaitingPlayer() {
				util.DebugFlag("room", str.CRoom, "[%s] stopped cleanup timer, players connected", r.ID)
				cleanupTimer.Stop()
			}

			// automatically ready bot players
			if r.players.HasBot() {
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
			// if room cancelled, halt immediately
			if control.Type == message.Cancel {
				return
			}
		}
	}
}
