package room

import (
	"time"

	"github.com/dechristopher/lioctad/channel"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
)

// handle waiting for players period
// waits for <roomExpiryTime> seconds if all players are disconnected and will
// proceed to clean up the room if exceeded
func (r *Instance) handleWaitingForPlayers() {
	cleanupTimer := time.NewTimer(time.Minute * 15)

	connectionListener := channel.Map[r.ID].Listen()
	defer channel.Map[r.ID].UnListen(connectionListener)

	util.DebugFlag("room", str.CRoom, "[%s] waiting for players", r.ID)

	for {
		select {
		case numPlayers := <-connectionListener:
			util.DebugFlag("room", str.CRoom, "[%s] room player count changed: %d", r.ID, numPlayers)
			// start cleanup timer if no players are connected
			if numPlayers == 0 {
				util.DebugFlag("room", str.CRoom, "[%s] no players connected, started timer", r.ID)
				cleanupTimer = time.NewTimer(roomExpiryTime)
				continue
			}

			// stop timer if one or more players are connected
			if numPlayers > 0 {
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
			// room expired, clean up
			util.DebugFlag("room", str.CRoom, "[%s] room expired, cleaning up", r.ID)
			err := r.event(EventPlayerAbandons)
			if err != nil {
				panic(err)
			}
			return
		}
	}
}
