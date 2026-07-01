package room

import (
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// autoRematchDelay is how long a finished bot game waits before automatically
// starting a rematch. It gives the player a moment to breathe and decide
// whether to rematch, leave, or just let the next game begin. The same value is
// sent to clients (GameOverPayload.AutoRematch) to drive the countdown, so it
// must remain the single source of truth for the delay.
const autoRematchDelay = 5 * time.Second

// rematchWindow is how long a finished human-vs-human game waits for both
// players to agree a rematch before the room closes. It is sent to clients
// (GameOverPayload.RematchWindow) to drive the visible countdown, so it must
// remain the single source of truth for the window.
const rematchWindow = 30 * time.Second

// rematchDisconnectGrace is the shortened rematch window applied once an
// opponent disconnects during a human-vs-human rematch window. A rematch needs
// both players present, so once one leaves we only briefly wait for a reconnect
// before closing the room instead of holding the remaining player for the full
// rematchWindow. The client is told via RematchUpdatePayload so its countdown
// retimes and reflects that the opponent left.
const rematchDisconnectGrace = 8 * time.Second

// handleGameOver handles game finalization and rematch prompts
func (r *Instance) handleGameOver() {
	// gameOverDone bounds the auto-rematch goroutine below to the lifetime of
	// this handler invocation. Closed on return so the goroutine can never
	// outlive the handler (leaking, or polluting the next game's rematch state)
	// and never blocks forever sending into controlChannel.
	gameOverDone := make(chan struct{})
	defer close(gameOverDone)

	// auto-rematch: in games against a bot, after a short delay agree the
	// rematch for both sides and nudge the routine so the next game starts on
	// its own. Human-vs-human games are not auto-rematched: each player must
	// request a rematch (via RequestRematch -> controlChannel), and the 30s
	// timeout below ends the room if they don't. All exits are guarded by
	// gameOverDone (handler returned) and r.done (room torn down).
	if r.HasBot() {
		go func() {
			select {
			case <-time.After(autoRematchDelay):
			case <-gameOverDone:
				return
			case <-r.done:
				return
			}

			// Don't auto-rematch a game the human never actually played. A socket
			// that is still connected but whose player never moved (an idle/
			// backgrounded tab, or someone who wandered off to watch the home-page
			// TV) would otherwise spin up another engine game for nobody. Let the
			// room end via the rematch timeout instead. A genuinely engaged player
			// has at least one move recorded, so this never blocks a wanted
			// rematch — and a human who explicitly clicks rematch still gets one
			// via the control loop below regardless.
			if !r.humanMovedThisGame() {
				util.DebugFlag("room", str.CRoom, "[%s] human made no move; skipping auto-rematch", r.ID)
				return
			}

			// Only auto-rematch while the human is present. If they closed the
			// tab (a disconnect, with no explicit rematch click), don't bounce
			// the room through a fresh game — and engine search — for nobody.
			// Wait for them to reconnect instead, and let the outer rematch
			// timeout end the room if they never do: it returns from the handler,
			// closing gameOverDone, which unblocks this goroutine. A transient
			// network blip therefore only delays the auto-rematch rather than
			// cancelling a rematch the player actually wanted. The listener is
			// registered lazily so the common (still-connected) case stays a
			// straight-through fast path.
			if !r.bothPlayersConnected() {
				util.DebugFlag("room", str.CRoom, "[%s] auto-rematch deferred, awaiting reconnect", r.ID)
				connectionListener := channel.Map.GetSockMap(r.ID).Listen()
				defer channel.Map.GetSockMap(r.ID).UnListen(connectionListener)
				for !r.bothPlayersConnected() {
					select {
					case <-connectionListener:
					case <-gameOverDone:
						return
					case <-r.done:
						return
					}
				}
			}

			// manually set rematch agreed for both colors
			r.stateMu.Lock()
			util.DoBothColors(func(color octad.Color) {
				r.rematch.Agree(color)
			})
			r.stateMu.Unlock()

			// trigger routine; controlChannel is buffered, but still guard the
			// send so a returned/torn-down room can't block this goroutine
			select {
			case r.controlChannel <- message.RoomControl{
				Type: message.Rematch,
				Ctx: channel.SocketContext{
					Channel: r.ID,
					MT:      1,
				},
			}:
			case <-gameOverDone:
			case <-r.done:
			}
		}()
	}

	// Rematch window: wait rematchWindow for a rematch before the room closes.
	// fullDeadline is the original window end; deadline is the live (possibly
	// shortened) one. Bounding shortening by fullDeadline means a flapping
	// opponent can never extend the window past its original length.
	fullDeadline := time.Now().Add(rematchWindow)
	deadline := fullDeadline
	rematchTimeout := time.NewTimer(rematchWindow)
	defer rematchTimeout.Stop()

	// publish the window's deadline so a (re)connecting client gets an accurate
	// remaining countdown via GameOverStateMessage. A bot game counts down to its
	// auto-rematch; a human game counts down the manual rematch window. The
	// presence arm below keeps this current when the human window is shortened or
	// restored.
	r.stateMu.Lock()
	if r.players.HasBot() {
		r.rematchDeadline = time.Now().Add(autoRematchDelay)
	} else {
		r.rematchDeadline = fullDeadline
	}
	r.stateMu.Unlock()

	// resetTimeout re-arms rematchTimeout to fire at the given deadline, draining
	// any pending fire first (the reset-without-drain hazard).
	resetTimeout := func(at time.Time) {
		if !rematchTimeout.Stop() {
			select {
			case <-rematchTimeout.C:
			default:
			}
		}
		d := time.Until(at)
		if d < 0 {
			d = 0
		}
		rematchTimeout.Reset(d)
	}

	// In human-vs-human games we watch player presence so the window can be
	// shortened the moment an opponent leaves (a rematch needs both players, so
	// there is no point holding the remaining player for the full window). Bot
	// games keep the fixed window: the auto-rematch goroutine above already
	// handles a disconnected human by deferring until they return. A nil channel
	// never fires in select, so the bot path simply skips this arm.
	var connectionListener channel.Listener
	if !r.HasBot() {
		connectionListener = channel.Map.GetSockMap(r.ID).Listen()
		defer channel.Map.GetSockMap(r.ID).UnListen(connectionListener)
	}
	// shortened guards against re-broadcasting / re-shortening on every presence
	// signal; we only act on the connected<->disconnected transition.
	shortened := false

	// broadcastRematchUpdate tells the remaining clients the window retimed (and
	// whether the opponent left), so their countdown follows the server.
	broadcastRematchUpdate := func(opponentLeft bool) {
		secs := int(time.Until(deadline).Round(time.Second).Seconds())
		if secs < 0 {
			secs = 0
		}
		proto.RematchUpdatePayload{
			Seconds:      secs,
			OpponentLeft: opponentLeft,
		}.Broadcast(channel.SocketContext{Channel: r.ID, MT: 1})
	}

	for {
		select {
		case <-rematchTimeout.C:
			// no rematch agreed to, clean up
			util.DebugFlag("room", str.CRoom, "[%s] no rematch, room over", r.ID)
			err := r.event(EventNoRematch)
			if err != nil {
				panic(err)
			}
			return
		// a player's presence changed: shorten the window when an opponent
		// leaves, restore it (bounded by the original deadline) if they return
		case <-connectionListener:
			bothConnected := r.bothPlayersConnected()
			switch {
			case !bothConnected && !shortened:
				shortened = true
				deadline = time.Now().Add(rematchDisconnectGrace)
				if deadline.After(fullDeadline) {
					deadline = fullDeadline
				}
				util.DebugFlag("room", str.CRoom, "[%s] opponent left, shortening rematch window", r.ID)
				resetTimeout(deadline)
				r.setRematchDeadline(deadline)
				broadcastRematchUpdate(true)
			case bothConnected && shortened:
				shortened = false
				deadline = fullDeadline
				util.DebugFlag("room", str.CRoom, "[%s] opponent returned, restoring rematch window", r.ID)
				resetTimeout(deadline)
				r.setRematchDeadline(deadline)
				broadcastRematchUpdate(false)
			}
			continue
		case control := <-r.controlChannel:
			if control.Type != message.Rematch {
				continue
			}

			// record rematch agreement and decide whether both sides agreed,
			// all under stateMu since rematch and players are also touched by
			// the auto-rematch goroutine above
			r.stateMu.Lock()
			util.DoBothColors(func(c octad.Color) {
				// track agreement for the player looked up via context
				if r.players[c] != nil && r.players[c].ID == control.Ctx.UID {
					r.rematch.Agree(c)
				}
				// auto-agree to rematch if this side is a bot
				if r.players[c] != nil && r.players[c].IsBot {
					r.rematch.Agree(c)
				}
			})
			agreed := r.rematch.Agreed()
			r.stateMu.Unlock()

			if !agreed {
				// one side asked for a rematch but the other hasn't yet: tell the
				// remaining clients so the opponent sees a "wants a rematch"
				// indicator. Only a real player click (a non-empty UID) is worth
				// surfacing; the bot auto-rematch path agrees both sides at once and
				// never lands here.
				if control.Ctx.UID != "" {
					proto.RematchUpdatePayload{
						Requested: control.Ctx.UID,
					}.Broadcast(channel.SocketContext{Channel: r.ID, MT: 1})
				}
				continue
			}

			if err := r.event(EventRematchAgreed); err != nil {
				panic(err)
			}

			// brief pause before the next game starts; bail early if the room
			// is being torn down so we don't linger
			t := time.NewTimer(time.Second)
			select {
			case <-t.C:
			case <-r.done:
				t.Stop()
				return
			}

			// flip the board and reset the game for the rematch, under the lock
			// so readers never see a half-swapped game pointer
			r.stateMu.Lock()
			r.flipBoardLocked()
			newGame, err := game.NewOctadGame(r.params.GameConfig)
			if err == nil {
				r.game = newGame
				// reset rematch flags for the next game over
				r.rematch = player.NewAgreement()
				// the new game has no human move yet; reset engagement so the
				// next game-over re-evaluates auto-rematch / idle-abandon fresh
				r.humanMoved = false
			}
			r.stateMu.Unlock()
			if err != nil {
				panic(err)
			}

			// discard any control message left buffered from this game-over (a
			// duplicate/early rematch click) so it can't be misread as agreement
			// in the next game's rematch window. See arch/DEPLOY_REMATCH_RACES.md.
			r.drainControlChannel()

			return
		}
	}
}
