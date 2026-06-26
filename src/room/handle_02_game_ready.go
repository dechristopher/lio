package room

import (
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/tv"
	"github.com/dechristopher/lio/util"
)

// handle waiting for white to make first move and start game
// waits for one minute before timing out and terminating game and room
func (r *Instance) handleGameReady() {
	// one-minute abandon timer after game start
	cleanupTimer := time.NewTimer(time.Minute)
	defer cleanupTimer.Stop()

	// listen for connection changes so we can defer the engine's first move
	// until the human opponent is actually present (see maybeRequestFirstMove).
	connectionListener := channel.Map.GetSockMap(r.ID).Listen()
	defer channel.Map.GetSockMap(r.ID).UnListen(connectionListener)

	// broadcast reset board state to all
	channel.Broadcast(r.CurrentGameStateMessage(false, true), channel.SocketContext{
		Channel: r.ID,
		MT:      1,
	})

	// announce the (re)started game to the home-page TV stream. This fires for
	// the first game and again for each rematch (the routine re-enters this
	// state), so a rematch streams its new game into the same TV slot.
	tv.Publish(r.tvEvent(tv.Start))

	util.DebugFlag("room", str.CRoom, "[%s] waiting for white to move", r.ID)

	// When the bot plays White it owns the first move, so nothing forces a human
	// action before an engine search is dispatched. Gate that search on the human
	// opponent being connected so we never burn a search for a game nobody is
	// watching (e.g. the player closed the tab during the auto-rematch window).
	// Human-as-White games request the engine move from makeMove instead, after a
	// real human move, so they need no gating here. The flag keeps this to a
	// single dispatch despite the primed listener and repeated connection events.
	engineToMove := r.botColor() == octad.White
	engineRequested := false
	maybeRequestFirstMove := func() {
		if !engineToMove || engineRequested || !r.bothPlayersConnected() {
			return
		}
		util.DebugFlag("room", str.CRoom, "[%s] engine making first move..", r.ID)
		r.requestEngineMove()
		engineRequested = true
	}
	maybeRequestFirstMove()

	for {
		select {
		case <-connectionListener:
			// human (re)connected: dispatch the deferred first move if we held it
			// TODO: guarantee this isn't a spectator
			maybeRequestFirstMove()
		case move := <-r.moveChannel:
			util.DebugFlag("room", str.CRoom, "[%s] got move %s from %s (%s / %s)", r.ID, move.Move.UOI, move.Player, r.game.White, r.game.Black)

			// don't allow moves out of order
			if !r.isTurn(move) {
				channel.Unicast(r.CurrentGameStateMessage(false, false), move.Ctx)
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] white (%s) trying to make first move", r.ID, move.Player)
			// start game clock on first move
			if r.game.Clock.State(true).IsPaused {
				util.DebugFlag("room", str.CRoom, "[%s] starting clock", r.ID)
				r.game.Clock.Start()
			}

			// make move and continue routine if move failed
			if ok := r.makeMove(move); !ok {
				util.DebugFlag("room", str.CRoom, "[%s] invalid first move, resetting clock", r.ID)
				r.game.Clock.Reset()

				// re-request engine first move
				if r.botColor() == octad.White {
					r.requestEngineMove()
				}
				continue
			}

			util.DebugFlag("room", str.CRoom, "[%s] white made first move, game (%s) starting", r.ID, r.game.ID)

			// transition game state to GameOngoing
			err := r.event(EventStartGame)
			if err != nil {
				panic(err)
			}

			return
		case <-cleanupTimer.C:
			r.abandoned = true
			// game expired, white timed out making first move
			util.DebugFlag("room", str.CRoom, "[%s] game expired, white timed out making first move, cleaning up", r.ID)
			err := r.event(EventPlayerAbandons)
			if err != nil {
				panic(err)
			}
			return
		}
	}
}
