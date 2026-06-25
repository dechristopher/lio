package room

import (
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// autoRematchDelay is how long a finished bot game waits before automatically
// starting a rematch. It gives the player a moment to breathe and decide
// whether to rematch, leave, or just let the next game begin. The same value is
// sent to clients (GameOverPayload.AutoRematch) to drive the countdown, so it
// must remain the single source of truth for the delay.
const autoRematchDelay = 5 * time.Second

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

	// 30 second timeout until rematch is unavailable
	rematchTimeout := time.NewTimer(30 * time.Second)
	defer rematchTimeout.Stop()

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
			}
			r.stateMu.Unlock()
			if err != nil {
				panic(err)
			}

			return
		}
	}
}
