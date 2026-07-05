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

// rematchWindow is how long a finished human-vs-human game waits for both
// players to agree a rematch before the room closes. It is sent to clients
// (GameOverPayload.RematchWindow) to drive the visible countdown, so it must
// remain the single source of truth for the window.
const rematchWindow = 30 * time.Second

// botAnalysisWindow is how long a finished bot game's room stays open before it
// is torn down. It is not a rematch window (bot rematch spins up a fresh room —
// see NewRoomVsComputer — rather than reusing this one) but an analysis grace:
// the player can review the finished game locally, and a reconnect within it
// still restores the result + move list from the server. There is no client
// countdown for it. Once it lapses the room closes; the player may keep analyzing
// locally (the client stops auto-reconnecting) and can still rematch into a new
// room.
const botAnalysisWindow = 2 * time.Minute

// rematchDisconnectGrace is the short window the room waits for a departed player
// to reconnect before closing. In a human-vs-human game it shortens the
// remaining rematch window once an opponent leaves (a rematch needs both players
// present); in a bot game it shortens the analysis window once the player leaves
// (no point holding the room for someone who is gone). The client is told via
// RematchUpdatePayload (human games) so its countdown retimes. A var only so
// tests can shorten it (like deployTimeout).
var rematchDisconnectGrace = 8 * time.Second

// matchInterludeWindow is the pause between games of an undecided race-to
// match: long enough to read the result overlay, short enough to keep the race
// moving. The next game starts automatically when it lapses — no rematch
// agreement. It is sent to clients (GameOverPayload.NextGameIn) to drive the
// visible countdown, so it must remain the single source of truth. A var only
// so tests can shorten it (like deployTimeout).
var matchInterludeWindow = 5 * time.Second

// handleGameOver handles game finalization and rematch prompts.
//
// An undecided race-to match holds no rematch negotiation at all: the next game
// auto-starts after a short interlude (handleMatchInterlude). Otherwise — a
// classic room, or a match whose race was just decided — a human-vs-human game
// holds rematchWindow for both players to agree a rematch (a fresh match, with
// scores reset, when the race was decided), shortened the moment an opponent
// leaves, and closes if it lapses. A bot game is not auto-rematched and its
// rematch is a fresh room (not this one), so this room simply stays open for
// botAnalysisWindow — long enough to review the game and survive a reconnect —
// with no countdown, shortened to the disconnect grace once the player leaves,
// then closes.
func (r *Instance) handleGameOver() {
	// an undecided race-to match auto-advances instead of negotiating a rematch
	if decided, _ := r.MatchDecided(); r.params.RaceTo > 0 && !decided {
		r.handleMatchInterlude()
		return
	}

	isBot := r.HasBot()

	// the open window: a manual rematch window for humans, an analysis grace for
	// bot games.
	window := rematchWindow
	if isBot {
		window = botAnalysisWindow
	}

	// fullDeadline is the window's original end; deadline is the live (possibly
	// shortened) one. Bounding shortening by fullDeadline means a flapping player
	// can never extend the window past its original length.
	fullDeadline := time.Now().Add(window)
	deadline := fullDeadline
	closeTimeout := time.NewTimer(window)
	defer closeTimeout.Stop()

	// stopTimeout stops the close timer, draining any pending fire first (the
	// reset-without-drain hazard).
	stopTimeout := func() {
		if !closeTimeout.Stop() {
			select {
			case <-closeTimeout.C:
			default:
			}
		}
	}
	// resetTimeout re-arms the close timer to fire at the given deadline.
	resetTimeout := func(at time.Time) {
		stopTimeout()
		d := time.Until(at)
		if d < 0 {
			d = 0
		}
		closeTimeout.Reset(d)
	}

	// publish the human window's deadline so a (re)connecting client gets an
	// accurate remaining countdown via GameOverStateMessage. Bot games have no
	// countdown (rematchDeadline stays zero), reflecting the analysis grace.
	r.stateMu.Lock()
	if isBot {
		r.rematchDeadline = time.Time{}
	} else {
		r.rematchDeadline = fullDeadline
	}
	r.stateMu.Unlock()

	// Watch player presence so the window can be shortened the moment a player
	// leaves — there is no point holding either a rematch window (needs both
	// players) or a bot analysis window (nobody left to analyze) for someone who
	// is gone. A reconnect within the grace restores the window.
	connectionListener := channel.Map.GetSockMap(r.ID).Listen()
	defer channel.Map.GetSockMap(r.ID).UnListen(connectionListener)
	// shortened guards against re-acting on every presence signal; we only act on
	// the connected<->disconnected transition.
	shortened := false

	// broadcastRematchUpdate tells the remaining clients the human window retimed
	// (and whether the opponent left), so their countdown follows the server. Bot
	// games have no countdown, so this is human-only.
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
		case <-closeTimeout.C:
			// the window lapsed (or a departed player's grace elapsed): close
			util.DebugFlag("room", str.CRoom, "[%s] no rematch, room over", r.ID)
			err := r.event(EventNoRematch)
			if err != nil {
				panic(err)
			}
			return
		// a player's presence changed: shorten the window when a player leaves,
		// restore it (bounded by the original deadline) if they return
		case <-connectionListener:
			bothConnected := r.bothPlayersConnected()
			switch {
			case !bothConnected && !shortened:
				shortened = true
				deadline = time.Now().Add(rematchDisconnectGrace)
				if deadline.After(fullDeadline) {
					deadline = fullDeadline
				}
				util.DebugFlag("room", str.CRoom, "[%s] player left finished game, shortening close window", r.ID)
				resetTimeout(deadline)
				if !isBot {
					r.setRematchDeadline(deadline)
					broadcastRematchUpdate(true)
				}
			case bothConnected && shortened:
				shortened = false
				deadline = fullDeadline
				util.DebugFlag("room", str.CRoom, "[%s] player returned to finished game, restoring close window", r.ID)
				resetTimeout(deadline)
				if !isBot {
					r.setRematchDeadline(deadline)
					broadcastRematchUpdate(false)
				}
			}
			continue
		case control := <-r.controlChannel:
			if control.Type != message.Rematch {
				continue
			}

			// record rematch agreement and decide whether both sides agreed, all
			// under stateMu since rematch and players are also touched elsewhere.
			// Bot games rematch into a fresh room client-side and never send this,
			// but the bot auto-agree is kept so a stray control still behaves.
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
				// surfacing.
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
			// so readers never see a half-swapped game pointer. In a race-to
			// room this window only ever runs with the race decided, so the
			// agreement starts a fresh match: scores and history reset too.
			r.stateMu.Lock()
			err := r.resetForNextGameLocked(r.params.RaceTo > 0)
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

// resetForNextGameLocked flips the board and swaps in a fresh game for the
// next game of the room's series, resetting all per-game state: the rematch
// and draw agreements, the engagement flag, and the published game-over
// deadlines. resetScore additionally clears both players' accumulated match
// score and per-game history — used when a decided race-to match restarts as
// a fresh match. The caller must hold stateMu and, on error, must not assume
// any state was reset (the game pointer is only swapped on success).
func (r *Instance) resetForNextGameLocked(resetScore bool) error {
	r.flipBoardLocked()
	newGame, err := game.NewOctadGame(r.params.GameConfig)
	if err != nil {
		return err
	}

	r.game = newGame
	// reset rematch flags for the next game over
	r.rematch = player.NewAgreement()
	// clear any draw-offer state so it can't carry into the next game
	r.draw = player.NewAgreement()
	r.drawOffer = octad.NoColor
	// the new game has no human move yet; reset engagement so the
	// next game-over re-evaluates idle-abandon fresh
	r.humanMoved = false
	// the finished game's published countdowns are void with it
	r.rematchDeadline = time.Time{}
	r.nextGameDeadline = time.Time{}

	if resetScore {
		r.players.ResetScores()
	}

	return nil
}

// handleMatchInterlude waits out the short pause between games of an undecided
// race-to match, then auto-advances to the next game — no rematch agreement is
// involved and no control traffic is honored (stray clicks accepted by
// RequestRematch's decided-outcome window are drained at the boundary). The
// interlude deadline was published (nextGameDeadline) by tryGameOver, so the
// game-over broadcast and any reconnect state carry the same countdown this
// handler waits on.
//
// Presence: the interlude does not chase every connection flap — the deadline
// simply checks that both players are present when it lapses. A missing player
// gets one rematchDisconnectGrace to return (a reconnect inside it advances
// immediately); still absent after the grace forfeits the match and the room
// is abandoned, with an explicit room-over notice since the mid-match game-over
// broadcast did not carry RoomOver.
func (r *Instance) handleMatchInterlude() {
	r.stateMu.Lock()
	deadline := r.nextGameDeadline
	if deadline.IsZero() {
		// defensive: tryGameOver publishes the deadline before transitioning
		// here; if it somehow didn't, fall back to a full interlude from now
		deadline = time.Now().Add(matchInterludeWindow)
		r.nextGameDeadline = deadline
	}
	r.stateMu.Unlock()

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	connectionListener := channel.Map.GetSockMap(r.ID).Listen()
	defer channel.Map.GetSockMap(r.ID).UnListen(connectionListener)

	// advance flips the board into the next game of the match and hands the
	// routine to handleGameReady (which sweeps the deploy channel and re-enters
	// the deploy/first-move flow).
	advance := func() {
		util.DebugFlag("room", str.CRoom, "[%s] match continues, starting next game", r.ID)

		r.stateMu.Lock()
		err := r.resetForNextGameLocked(false)
		r.stateMu.Unlock()
		if err != nil {
			panic(err)
		}

		// discard any stray rematch click buffered during the finished game so
		// it can't be misread as agreement in a later game-over window
		r.drainControlChannel()

		if err := r.event(EventNextGame); err != nil {
			panic(err)
		}
	}

	graceArmed := false
	for {
		select {
		case <-timer.C:
			if r.bothPlayersConnected() {
				advance()
				return
			}
			if !graceArmed {
				// a player is missing at the deadline: hold the advance for one
				// grace so a momentary drop doesn't forfeit the match
				graceArmed = true
				util.DebugFlag("room", str.CRoom, "[%s] player missing at match interlude end, holding for grace", r.ID)
				timer.Reset(rematchDisconnectGrace)
				continue
			}

			// still missing after the grace: the match is forfeited. The
			// mid-match game-over broadcast carried no RoomOver, so tell the
			// clients the room is closing before the abandon transition (whose
			// handleRoomOver path is silent once a game has finished).
			util.DebugFlag("room", str.CRoom, "[%s] player left mid-match, room over", r.ID)
			payload := proto.GameOverPayload{
				Status:   "PLAYER LEFT THE MATCH - MATCH OVER",
				RoomOver: true,
			}
			channel.Broadcast(payload.Marshal(), channel.SocketContext{
				Channel: r.ID,
				MT:      1,
			})

			r.abandoned = true
			if err := r.event(EventPlayerAbandons); err != nil {
				panic(err)
			}
			return
		case <-connectionListener:
			// presence changed: a return during the held grace advances right
			// away rather than waiting the grace out
			if graceArmed && r.bothPlayersConnected() {
				advance()
				return
			}
		case <-r.controlChannel:
			// no rematch negotiation happens mid-match; swallow stray controls
			// so the buffer can't fill with clicks meant for a decided game
		}
	}
}
