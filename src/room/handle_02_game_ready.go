package room

import (
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/tv"
	"github.com/dechristopher/lio/util"
)

// handle waiting for white to make first move and start game
// waits for one minute before timing out and terminating game and room
func (r *Instance) handleGameReady() {
	// when the deploy pre-game is enabled, hand off to the blind deploy phase
	// instead of waiting for white's first move. The deploy handler broadcasts
	// its own start state, assembles the game, and transitions to GameOngoing.
	if r.params.Deploy {
		// sweep any straggler submission left buffered from a previous deploy
		// phase (one that landed after that phase stopped reading) so it cannot
		// be consumed as a legitimate submission in the phase about to start.
		// This must happen BEFORE EventStartDeploy fires: SubmitDeploy rejects
		// sends outside StateDeploy, so at this point the buffer can only hold
		// stragglers — draining inside handleDeploy instead would race (and
		// discard) a real submission arriving right after the transition.
		r.drainDeployChannel()

		util.DebugFlag("room", str.CRoom, "[%s] starting deploy phase", r.ID)
		if err := r.event(EventStartDeploy); err != nil {
			panic(err)
		}
		return
	}

	// one-minute abandon timer after game start
	cleanupTimer := time.NewTimer(time.Minute)
	defer cleanupTimer.Stop()

	// stop/reset helpers for the cleanup timer, draining any pending fire first
	// (the reset-without-drain hazard). Only the casual logic below retimes
	// the timer; classic games keep the single armed minute.
	stopCleanup := func() {
		if !cleanupTimer.Stop() {
			select {
			case <-cleanupTimer.C:
			default:
			}
		}
	}
	resetCleanup := func(d time.Duration) {
		stopCleanup()
		cleanupTimer.Reset(d)
	}

	// listen for connection changes so we can defer the engine's first move
	// until the human opponent is actually present (see maybeRequestFirstMove).
	connectionListener := channel.Map.GetSockMap(r.ID).Listen()
	defer channel.Map.GetSockMap(r.ID).UnListen(connectionListener)

	// A casual game is not first-move time-boxed: its players may think over
	// the opening indefinitely, so the fixed minute above would wrongly expire
	// a connected session. Presence drives the timer instead — disarmed while
	// everyone is connected, re-armed to the room's disconnect timeout the
	// moment a seat drops (the disconnect cancel that keeps abandoned casual
	// rooms from piling up; a casual bot game gets the short timeout, a human
	// game the standard reconnect tolerance). The room only enters this state
	// once players have connected (the waiting state gates on presence), so an
	// on-entry vacancy is a real disconnect, not someone still loading.
	casualTimeout := r.casualDisconnectTimeout()
	syncCasualCleanup := func() {
		if !r.params.Casual {
			return
		}
		if r.bothPlayersConnected() {
			stopCleanup()
		} else {
			resetCleanup(casualTimeout)
		}
	}
	syncCasualCleanup()

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
	// watching (e.g. the player left before the new game began).
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
			// casual rooms retime the cleanup timer off presence
			syncCasualCleanup()
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
			// a reconnect can race the fire: don't expire a casual room whose
			// players are present again (the pending listener event re-syncs the
			// timer next loop). Timed games keep the hard first-move box.
			if r.params.Casual && r.bothPlayersConnected() {
				continue
			}
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
