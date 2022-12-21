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
func (r *Instance) handleWaitingForPlayers() {
	cleanupTimer := time.NewTimer(lobbyExpiryTime)
	defer cleanupTimer.Stop()

	waitingRoom := channel.Map.GetSockMap(fmt.Sprintf("%s%s", wait, r.ID))
	connectionListener := waitingRoom.Listen()
	defer waitingRoom.UnListen(connectionListener)

	util.DebugFlag("room", str.CRoom, "[%s] waiting for players", r.ID)

	for {
		select {
		case numConnections := <-connectionListener:
			util.DebugFlag("room", str.CRoom, "[%s] lobby player count changed: %d", r.ID, numConnections)
			// immediately start the game if one of the players is a bot
			if r.players.HasBot() {
				util.DebugFlag("room", str.CRoom, "[%s] room has bot, game ready", r.ID)
				err := r.event(EventPlayersConnected)
				if err != nil {
					panic(err)
				}
				return
			}
		case <-cleanupTimer.C:
			r.abandoned = true
			// room expired, clean up
			util.DebugFlag("room", str.CRoom, "[%s] lobby expired, cleaning up", r.ID)
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

			if control.Type == message.Join {
				util.DebugFlag("room", str.CRoom, "[%s] player has joined", r.ID)
				err := r.event(EventPlayersConnected)
				if err != nil {
					panic(err)
				}
				return
			}
		}
	}
}
