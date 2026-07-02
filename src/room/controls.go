package room

import (
	"github.com/dechristopher/octad/v2"
	"github.com/looplab/fsm"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/dispatch"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// RequestResign enqueues a resignation on behalf of the requesting player. It is
// called from the WS read loop, so it validates that the request comes from a
// seated player while a game is ongoing and never blocks the caller: the
// controlChannel is buffered and the send is non-blocking (the player can click
// again if it somehow drops). The game-ongoing handler applies it.
func (r *Instance) RequestResign(meta channel.SocketContext) {
	if r.State() != StateGameOngoing || r.GameState() != octad.NoOutcome {
		return
	}

	// only seated players may resign
	r.stateMu.Lock()
	_, color := r.players.Lookup(meta.UID)
	r.stateMu.Unlock()
	if color == octad.NoColor {
		return
	}

	r.sendControl(message.RoomControl{Type: message.Resign, Player: meta.UID, Ctx: meta})
}

// RequestDraw enqueues a draw offer (or acceptance) on behalf of the requesting
// player. Like RequestResign it validates a seated player during an ongoing game
// and never blocks the caller. The game-ongoing handler records the agreement
// and, against a bot, asks the engine whether to accept.
func (r *Instance) RequestDraw(meta channel.SocketContext) {
	if r.State() != StateGameOngoing || r.GameState() != octad.NoOutcome {
		return
	}

	// only seated players may offer/accept a draw
	r.stateMu.Lock()
	_, color := r.players.Lookup(meta.UID)
	r.stateMu.Unlock()
	if color == octad.NoColor {
		return
	}

	r.sendControl(message.RoomControl{Type: message.Draw, Player: meta.UID, Ctx: meta})
}

// sendControl pushes an in-game control onto the buffered controlChannel without
// ever blocking the WS read loop: it selects on room teardown and drops on a
// full buffer. During an ongoing game the controlChannel has no other producer
// (rematch is only accepted at game-over, cancel only while waiting), so the two
// buffer slots comfortably hold a resign/draw pair; a dropped control just means
// the player clicks again.
func (r *Instance) sendControl(control message.RoomControl) {
	select {
	case r.controlChannel <- control:
	case <-r.done:
	default:
		util.DebugFlag("room", str.CRoom, "[%s] dropped control %d from %s: buffer full",
			r.ID, control.Type, control.Player)
	}
}

// handleGameControl processes an in-game control (resign or draw) received while
// the game is ongoing. It returns whether the game ended and, if so, the FSM
// transition event to fire. Rematch/cancel controls are not produced in this
// state and are ignored.
func (r *Instance) handleGameControl(control message.RoomControl) (bool, *fsm.EventDesc) {
	switch control.Type {
	case message.Resign:
		return r.resignControl(control)
	case message.Draw:
		return r.drawControl(control)
	default:
		return false, nil
	}
}

// resignControl resigns the requesting player's color and ends the game.
func (r *Instance) resignControl(control message.RoomControl) (bool, *fsm.EventDesc) {
	r.stateMu.Lock()
	_, color := r.players.Lookup(control.Ctx.UID)
	if color == octad.NoColor || r.game.Outcome() != octad.NoOutcome {
		r.stateMu.Unlock()
		return false, nil
	}
	r.game.Resign(color)
	r.stateMu.Unlock()

	util.DebugFlag("room", str.CRoom, "[%s] %s resigned", r.ID, color)
	return r.tryGameOver(control.Ctx, false)
}

// drawControl records a draw offer (or acceptance) from the requesting player.
// If both seats have now offered, the game is drawn by agreement. Otherwise the
// offer becomes the standing offer: it is broadcast so a human opponent can
// accept, and against a bot the engine is asked to accept or decline.
func (r *Instance) drawControl(control message.RoomControl) (bool, *fsm.EventDesc) {
	r.stateMu.Lock()
	_, color := r.players.Lookup(control.Ctx.UID)
	if color == octad.NoColor || r.game.Outcome() != octad.NoOutcome {
		r.stateMu.Unlock()
		return false, nil
	}

	// re-offering while our own offer already stands is a no-op
	if r.drawOffer == color {
		r.stateMu.Unlock()
		return false, nil
	}

	// mark this side; the Agreement accumulates, so a click that answers a
	// standing offer from the other side completes it (both sides marked)
	r.draw.Agree(color)
	agreed := r.draw.Agreed()
	botColor := r.players.GetBotColor()
	if !agreed {
		r.drawOffer = color
	}
	r.stateMu.Unlock()

	if agreed {
		return r.applyAgreedDraw(control.Ctx)
	}

	// surface the standing offer: the offerer's client shows a pending state, the
	// opponent's an "accept draw" affordance (each keys off By vs its own uid)
	proto.DrawOfferPayload{By: control.Ctx.UID}.Broadcast(channel.SocketContext{Channel: r.ID, MT: 1})

	// against the bot there is no human to accept: ask the engine to decide
	if botColor == color.Other() {
		r.requestEngineDraw()
	}
	return false, nil
}

// applyAgreedDraw draws the game by agreement and ends it. The caller has already
// established that both sides agreed (or the bot accepted on the human's behalf).
func (r *Instance) applyAgreedDraw(meta channel.SocketContext) (bool, *fsm.EventDesc) {
	r.stateMu.Lock()
	if r.game.Outcome() != octad.NoOutcome {
		r.stateMu.Unlock()
		return false, nil
	}
	if err := r.game.Draw(octad.DrawOffer); err != nil {
		r.stateMu.Unlock()
		util.Error(str.CRoom, "[%s] draw by agreement failed: %s", r.ID, err.Error())
		return false, nil
	}
	r.drawOffer = octad.NoColor
	r.draw = player.NewAgreement()
	r.stateMu.Unlock()

	util.DebugFlag("room", str.CRoom, "[%s] draw agreed", r.ID)
	return r.tryGameOver(meta, false)
}

// requestEngineDraw asks the engine dispatcher whether the bot should accept the
// standing draw offer, delivering the verdict on drawEvalChannel. It mirrors
// requestEngineMove: the search runs off the room routine so the game-ongoing
// select stays responsive, and the response channel is buffered so the worker's
// send never blocks even if the game ends first.
func (r *Instance) requestEngineDraw() {
	r.stateMu.Lock()
	req := dispatch.DrawRequest{
		GameID:          r.game.ID,
		OFEN:            r.game.OFEN(),
		Depth:           r.calcDepthLocked(r.game.ToMove),
		ResponseChannel: r.drawEvalChannel,
		Done:            r.done,
	}
	r.stateMu.Unlock()

	go dispatch.SubmitDraw(req)
}

// handleDrawEval applies the engine's verdict on a bot draw offer. A verdict for
// a stale game/position (a move landed since the offer, clearing drawOffer) is
// dropped silently. A decline clears the standing offer and tells clients; an
// accept draws the game by agreement and ends it.
func (r *Instance) handleDrawEval(eval *message.RoomDrawEval) (bool, *fsm.EventDesc) {
	r.stateMu.Lock()
	pending := r.drawOffer != octad.NoColor
	fresh := eval.GameID == r.game.ID && eval.OFEN == r.game.OFEN() &&
		r.game.Outcome() == octad.NoOutcome
	r.stateMu.Unlock()

	if !pending || !fresh {
		// the offer was already withdrawn by a move, or this verdict is for a
		// prior game/position; drop it (the client cleared its affordance on the
		// move that withdrew the offer).
		return false, nil
	}

	if !eval.Accept {
		r.stateMu.Lock()
		r.drawOffer = octad.NoColor
		r.draw = player.NewAgreement()
		r.stateMu.Unlock()
		util.DebugFlag("room", str.CRoom, "[%s] bot declined draw", r.ID)
		proto.DrawOfferPayload{Declined: true}.Broadcast(channel.SocketContext{Channel: r.ID, MT: 1})
		return false, nil
	}

	return r.applyAgreedDraw(channel.SocketContext{Channel: r.ID, MT: 1})
}
