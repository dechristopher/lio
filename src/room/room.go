package room

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dechristopher/octad"
	"github.com/looplab/fsm"

	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/channel/handlers"
	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/dispatch"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/lag"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/store"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/variant"
	"github.com/dechristopher/lio/www/ws/proto"
)

var gamePub = bus.NewPublisher("game", game.Channel)

// Type of room channel
type Type string

const (
	room    Type = ""
	waiting Type = "wait/"
)

var roomChannelTypes = []Type{
	room,
	waiting,
}

// rooms is a mapping from room ID to instance. It is accessed concurrently
// by HTTP/WS handler goroutines (Get/Create) and room routines (cleanup),
// so it must be a sync.Map to avoid fatal concurrent map access.
var rooms = &sync.Map{}

// Get room instance by id
func Get(id string) (*Instance, error) {
	instance, ok := rooms.Load(id)
	if !ok {
		return nil, ErrNoRoom{ID: id}
	}
	return instance.(*Instance), nil
}

// Count returns the number of active rooms
func Count() int {
	count := 0
	rooms.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// Instance is a struct that represents an ongoing match between
// two players, controlled by a finite state machine
type Instance struct {
	ID      string
	creator string
	params  Params
	game    *game.OctadGame

	stateMachine *fsm.FSM

	stateChannel   chan State
	moveChannel    chan *message.RoomMove
	controlChannel chan message.RoomControl

	// done is closed exactly once by cleanup when the room routine exits.
	// Senders into the room's channels select on it so they can never block
	// forever once the room is being torn down.
	done chan struct{}

	// stateMu guards the mutable game-state fields that are touched by more
	// than one goroutine: game (both the pointer, which is swapped on rematch,
	// and the octad.Game it points at), players (populated by Join from an HTTP
	// goroutine), and rematch (written by the auto-rematch goroutine). The room
	// routine is the sole writer of game contents, but HTTP/WS handler
	// goroutines read them (CurrentGameStateMessage, IsReady, GenTemplatePayload,
	// ...) and Join writes players, so every access must be synchronized.
	//
	// The octad library lazily caches move generation inside the Game, so even
	// a "read" can mutate it — hence an exclusive Mutex rather than an RWMutex.
	//
	// Convention: methods suffixed `Locked` assume the caller already holds
	// stateMu; every other method that touches the guarded fields acquires it
	// itself. Critical sections are kept small and are always released before a
	// network broadcast or a blocking clock operation so the lock never gates
	// I/O. cancelled and abandoned are intentionally not guarded here: they are
	// only ever touched by the room routine goroutine.
	stateMu sync.Mutex

	players player.Players
	rematch player.Agreement

	joinToken   string // token to control joining challenges
	cancelToken string // token to control cancelling challenges

	abandoned bool
	cancelled bool
}

// Params for room Instance creation
type Params struct {
	Creator    string
	Players    player.Players
	GameConfig game.OctadGameConfig
}

// NewParams returns a new parameters object configured
// using the given variant
func NewParams(creatorId string, variant variant.Variant) Params {
	return Params{
		Creator: creatorId,
		Players: make(player.Players),
		GameConfig: game.OctadGameConfig{
			Variant: variant,
		},
	}
}

// Create a room instance from the given parameters
func Create(params Params) (*Instance, error) {
	// make sure both players are configured
	if len(params.Players) != 2 {
		return nil, ErrBadParamsPlayers{}
	}

	// TODO no support for two bots at the moment
	// P1 must be a human for P2 to be a bot
	// (internal engine vs internal engine)
	if util.BothColors(func(c octad.Color) bool {
		return params.Players[c].IsBot
	}) {
		return nil, ErrBadParamsTwoBots{}
	}

	roomId := config.GenerateCode(7, config.Base58)

	for {
		if _, exists := rooms.Load(roomId); !exists {
			break
		}
		roomId = config.GenerateCode(7, config.Base58)
	}

	r := &Instance{
		ID:           roomId,
		creator:      params.Creator,
		stateMachine: newStateMachine(),
		params:       params,

		stateChannel: make(chan State, 1),
		moveChannel:  make(chan *message.RoomMove),
		// buffered for 2 so two near-simultaneous rematch requests (both human
		// players, or the bot auto-rematch plus a human click) can never drop a
		// non-blocking send; each rematch button disables after one click, so
		// no single client floods it
		controlChannel: make(chan message.RoomControl, 2),

		done: make(chan struct{}),

		players: params.Players,
		rematch: player.Agreement{},

		cancelToken: config.GenerateCode(12),
	}

	// randomize join token before anybody joins to prevent abuse
	r.NewJoinToken()

	// Keep track of all channels for off-rpc broadcasts
	// Create new SockMaps and track them under the channel key
	// for each room channel type
	for _, channelType := range roomChannelTypes {
		channel.Map.GetSockMap(fmt.Sprintf("%s%s", channelType, r.ID))
	}

	// handle crowd messages for primary room channel
	go handlers.HandleCrowd(r.ID)

	// populate the game config and create the initial game. No concurrency
	// exists yet (the routine has not started and the room is not yet in the
	// rooms map), but we hold stateMu to honor the locking convention.
	r.stateMu.Lock()
	r.populateGameConfigLocked()
	var err error
	r.game, err = game.NewOctadGame(r.params.GameConfig)
	r.stateMu.Unlock()
	if err != nil {
		return nil, err
	}

	// begin room routine
	err = r.init()
	if err != nil {
		return nil, err
	}

	// store room in rooms map
	rooms.Store(r.ID, r)

	// log room creation
	util.Info(str.CRoom, "[%s] room created by uid %s", r.ID, params.Creator)

	return r, nil
}

// init begins the room routine and initializes the room state
func (r *Instance) init() error {
	err := r.event(EventRoomInitialized)
	if err != nil {
		return err
	}

	go r.routine()
	return nil
}

// routine for handling all room operations after creation
func (r *Instance) routine() {
	// recover panicked room routines
	defer func() {
		err := recover()
		if err != nil {
			util.Error(str.CRoom, "[%s] recovered panicked room routine: %v", r.ID, err)
		}
	}()
	// defer room cleanup, still runs in case of a panic, thanks go
	defer r.cleanup()

	for {
		util.DebugFlag("room", str.CRoom, "[%s] room state transition - %s", r.ID, r.State())
		switch r.State() {
		case StateWaitingForPlayers:
			r.handleWaitingForPlayers()
		case StateGameReady:
			r.handleGameReady()
		case StateGameOngoing:
			r.handleGameOngoing()
		case StateGameOver:
			r.handleGameOver()
		case StateRoomOver:
			// housekeeping items go here
			r.handleRoomOver()
			return
		default:
			panic("invalid or unknown room state")
		}

		// exit routine if room has been cancelled
		if r.cancelled {
			return
		}
	}
}

// cleanup finishes, closes, and finalizes the room. It runs exactly once,
// from the room routine's deferred call, so the close(r.done) is safe.
func (r *Instance) cleanup() {
	util.DebugFlag("room", str.CRoom, "[%s] cleaning up", r.ID)
	// release any goroutines blocked sending into the room's channels
	// (SendMove / Cancel / engine dispatcher) before tearing anything down
	close(r.done)
	// clean up all existing room channels by type
	for _, channelType := range roomChannelTypes {
		c := fmt.Sprintf("%s%s", channelType, r.ID)
		if _, ok := channel.Map.Load(c); ok {
			channel.Map.GetSockMap(c).Cleanup()
		}
	}
	// delete room instance from rooms map
	rooms.Delete(r.ID)
}

// event runs a state machine transition using the given EventDesc and args
func (r *Instance) event(event fsm.EventDesc, args ...interface{}) error {
	err := r.stateMachine.Event(context.TODO(), event.Name, args)
	if err != nil {
		return err
	}
	return nil
}

// flipBoardLocked flips the player color and repopulates the game config from
// parameters ahead of a rematch. The caller must hold stateMu (it mutates
// players and params).
func (r *Instance) flipBoardLocked() {
	// change sides
	r.players.FlipColor()

	r.params.GameConfig.White = ""
	r.params.GameConfig.Black = ""

	// repopulate game config values from parameters
	r.populateGameConfigLocked()
}

// populateGameConfigLocked copies relevant parameter values to the game config
// parameter before generating a new game during init, or when flipping the
// board. The caller must hold stateMu (it reads players and writes params).
func (r *Instance) populateGameConfigLocked() {
	if r.params.GameConfig.White == "" && r.players[octad.White] != nil {
		r.params.GameConfig.White = r.players[octad.White].ID
	}

	if r.params.GameConfig.Black == "" && r.players[octad.Black] != nil {
		r.params.GameConfig.Black = r.players[octad.Black].ID
	}
}

// IsReady returns true if the room is ready for games to be played.
func (r *Instance) IsReady() bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	// call the unlocked players helpers directly (not r.HasBot, which would
	// re-acquire stateMu and deadlock)
	hasTwoPlayers, _ := r.players.HasTwoPlayers()
	return hasTwoPlayers || r.players.HasBot()
}

// HandlePreGame sets flags and token info in the RoomTemplatePayload if the
// room is in the pre-game state
func (r *Instance) HandlePreGame(uid string, payload *message.RoomTemplatePayload) {
	if !r.IsReady() {
		payload.IsCreator = r.IsCreator(uid)
		payload.IsJoining = !payload.IsCreator

		// set cancel token in payload
		if payload.IsCreator {
			payload.CancelToken = r.CancelToken()
		}
		// set new join token in payload
		if payload.IsJoining {
			payload.JoinToken = r.NewJoinToken()
		}
	}
}

// Cancel will cancel the room if not past waiting for players state.
// The cancelled flag is set by the room routine itself (on receipt of the
// control message), never here, to avoid a cross-goroutine data race and to
// prevent a cancel that loses the race against game start from tearing down
// an in-progress game.
func (r *Instance) Cancel() bool {
	// only allow room cancellation in the waiting state
	if r.State() != StateWaitingForPlayers {
		return false
	}

	// emit control message to signal room routine handler to exit.
	// controlChannel is buffered, so this does not require the routine to be
	// parked on a receive; the done case guards against a torn-down room.
	select {
	case r.controlChannel <- message.RoomControl{
		Type: message.Cancel,
		Ctx: channel.SocketContext{
			Channel: r.ID,
			MT:      1,
		},
	}:
		return true
	case <-r.done:
		return false
	}
}

// CanJoin returns true if the given player by uid can participate in the match
// either as a player or a spectator
func (r *Instance) CanJoin(uid string) (asPlayer, asSpectator bool) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	hasPlayers, _ := r.players.HasTwoPlayers()

	// force into spectator mode if match has both players
	if hasPlayers && !r.players.IsPlayer(uid) {
		// TODO track spectator somehow
		return false, true
	}

	// player can return if P1, or join as P2
	return true, false
}

// Join attempts to join the room as a human player. It is called from HTTP
// handler goroutines and may race other joins as well as the room routine, so
// the whole check-and-populate is done under stateMu: the HasTwoPlayers test
// and the map write must be atomic or two players following the same join link
// could both pass the test and corrupt the players map.
func (r *Instance) Join(uid, joinToken string) bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	// validate provided join token
	if joinToken != r.joinToken {
		return false
	}

	// if room established with both players
	hasPlayers, missing := r.players.HasTwoPlayers()

	// if second player joining
	if !hasPlayers && missing != octad.NoColor {
		// set missing player
		r.players[missing] = &player.Player{
			ID: uid,
		}

		// set internal game instance state
		if missing == octad.White {
			r.game.White = uid
		} else {
			r.game.Black = uid
		}

		// joined properly
		return true
	}

	// game already has both players
	return false
}

// NotifyWaiting notifies the waiting player(s) that games have begun
// by sending redirect messages to the room
func (r *Instance) NotifyWaiting() {
	waitingChannelName := fmt.Sprintf("%s%s", waiting, r.ID)
	// ensure channel exists before notifying players
	if waitingChannel := channel.Map.GetSockMap(waitingChannelName); waitingChannel != nil {
		meta := channel.SocketContext{
			Channel: waitingChannelName,
			MT:      1,
		}
		// create redirect message
		redir := proto.RedirectMessage{
			Location: fmt.Sprintf("/%s", r.ID),
		}
		// broadcast redirect message
		channel.Broadcast(redir.Marshal(), meta)
	}
}

// NewJoinToken returns a new randomized join token for the
// joining user. Updates the stored join token so other users
// already on the join page can't also accept a challenge
func (r *Instance) NewJoinToken() string {
	r.joinToken = config.GenerateCode(12)
	return r.joinToken
}

// CancelToken returns the cancelToken to the challenging player
// such that they may cancel the challenge before games start
func (r *Instance) CancelToken() string {
	return r.cancelToken
}

// State returns the current room state
func (r *Instance) State() State {
	return State(r.stateMachine.Current())
}

// IsCreator returns true if the given player by ID is the creator of the room
func (r *Instance) IsCreator(id string) bool {
	return r.creator == id
}

// HasBot returns true if the room is configured with a bot player
func (r *Instance) HasBot() bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.players.HasBot()
}

// Game returns the game container instance.
//
// NOTE: this hands out the raw *game.OctadGame pointer, which is not safe to
// read concurrently with the room routine mutating it. It is intended for
// single-threaded/internal use only; cross-goroutine callers should go through
// CurrentGameStateMessage / GameState, which snapshot under stateMu.
func (r *Instance) Game() *game.OctadGame {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.game
}

// botColor returns the color the configured bot is playing (or NoColor). It is
// safe to call from the room routine outside a locked section (e.g. when a Join
// may still be populating the players map).
func (r *Instance) botColor() octad.Color {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.players.GetBotColor()
}

// playerInfo returns a stable (id, isBot) snapshot for the given color, taken
// under stateMu so it never races a Join populating the players map. ID and
// IsBot are immutable once a player is seated, so the returned values stay valid
// after the lock is released (connection state is then read from the
// independently-synchronized channel layer).
func (r *Instance) playerInfo(color octad.Color) (id string, isBot bool) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	if p := r.players[color]; p != nil {
		return p.ID, p.IsBot
	}
	return "", false
}

// bothPlayersConnected reports whether every human seat currently holds a live
// connection on the room channel; a bot seat counts as always-connected. It is
// the presence primitive shared by the abandon detection (handleGameOngoing) and
// the engine-move gating (handleGameReady): we never dispatch an engine search,
// or keep a game alive, for a position nobody is watching. playerInfo locks
// stateMu and Connected locks the channel layer independently, so this never
// nests the two locks.
func (r *Instance) bothPlayersConnected() bool {
	return util.BothColors(func(color octad.Color) bool {
		id, isBot := r.playerInfo(color)
		if isBot {
			return true
		}
		return channel.Map.GetSockMap(r.ID).Connected(id)
	})
}

// SendMove writes a move to the room's moveChannel to be consumed by the
// room routine. moveChannel is only drained while the game is ready or
// ongoing, so the send selects on r.done to guarantee it can never block a
// caller goroutine (a WS read loop) forever once the room is torn down.
func (r *Instance) SendMove(move *message.RoomMove) {
	// prevent first moves before moves are allowed to be played
	if r.State() == StateWaitingForPlayers {
		return
	}

	select {
	case r.moveChannel <- move:
	case <-r.done:
		// room is being torn down; drop the move
	}
}

// RequestRematch enqueues a rematch agreement on behalf of the requesting
// player. It is called from the WS read loop, so it validates that the request
// comes from a seated player while the game is over, and never blocks the
// caller: if the control buffer is full it drops the request (the player can
// click again, and bot games auto-rematch regardless). The game-over handler
// applies the agreement (and auto-agrees a bot opponent).
func (r *Instance) RequestRematch(meta channel.SocketContext) {
	// only meaningful once the game is over, which is also the only window the
	// game-over handler is reading controlChannel
	if r.State() != StateGameOver {
		return
	}

	// only seated players may request a rematch
	r.stateMu.Lock()
	_, color := r.players.Lookup(meta.UID)
	r.stateMu.Unlock()
	if color == octad.NoColor {
		return
	}

	select {
	case r.controlChannel <- message.RoomControl{
		Type: message.Rematch,
		Ctx:  meta,
	}:
	default:
		// control buffer full or handler not reading; drop the request
	}
}

// CurrentGameStateMessage returns the octad position, marshalled as a move
// payload. It is called from WS handler goroutines as well as the room routine,
// so it snapshots the game under stateMu.
func (r *Instance) CurrentGameStateMessage(addLast bool, gameStart bool) []byte {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.currentGameStateMessageLocked(addLast, gameStart)
}

// currentGameStateMessageLocked builds the current board-state payload. The
// caller must hold stateMu (it reads the game and players).
func (r *Instance) currentGameStateMessageLocked(addLast bool, gameStart bool) []byte {
	curr := proto.MovePayload{
		Clock:     r.currentClockLocked(),
		OFEN:      r.game.OFEN(),
		MoveNum:   len(r.game.Moves()) / 2,
		Check:     r.game.Position().InCheck(),
		Moves:     r.game.MoveHistory(),
		Latency:   clock.ToCTime(0),
		White:     r.players[octad.White].ID,
		Black:     r.players[octad.Black].ID,
		Score:     r.players.ScoreMap(),
		GameStart: gameStart,
	}

	// set legal moves if we're in GameReady or GameOngoing
	// to prevent first moves before moves are allowed to be played
	if r.State() != StateWaitingForPlayers {
		curr.ValidMoves = r.game.LegalMoves()
	}

	// add last move SAN if enabled
	if addLast {
		curr.SAN = r.getSANLocked()
	}

	return curr.Marshal()
}

// currentClockLocked returns the current clock state via a ClockPayload. The
// caller must hold stateMu (it reads the game pointer); the clock itself is
// independently synchronized.
func (r *Instance) currentClockLocked() proto.ClockPayload {
	state := r.game.Clock.State(true)
	return proto.ClockPayload{
		Control: r.game.Variant.Control.Time.Centi(),
		Black:   state.BlackTime.Centi(),
		White:   state.WhiteTime.Centi(),
		Lag:     clock.ToCTime(lag.Move.Get()).Centi(),
	}
}

// getSANLocked returns the last move in algebraic notation. The caller must
// hold stateMu (it reads the game's move/position history).
func (r *Instance) getSANLocked() string {
	if len(r.game.Positions()) > 1 {
		pos := r.game.Positions()[len(r.game.Positions())-2]
		move := r.game.Moves()[len(r.game.Moves())-1]
		return octad.AlgebraicNotation{}.Encode(pos, move)
	}

	return ""
}

// isTurn returns true to ensure moves are received and processed only during
// the given player's turn. It is called by the room routine before makeMove and
// snapshots players + the side to move under stateMu.
func (r *Instance) isTurn(move *message.RoomMove) bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	// handle bot turns
	if move.Ctx.IsBot {
		// TODO no P1 bot support at the moment
		if r.players.GetBotColor() == r.game.ToMove {
			return true
		}
	}

	// lookup player color by ID
	_, playerColor := r.players.Lookup(move.Ctx.UID)
	return playerColor == r.game.ToMove
}

// makeMove attempts to make the given move, transition game state, and notify
// all channel connections of the game state. It is called only by the room
// routine. The actual mutation of the game runs in a small stateMu critical
// section; the broadcasts and the engine request happen after the lock is
// released (they re-acquire it via the self-locking helpers) so the lock never
// gates network I/O.
func (r *Instance) makeMove(move *message.RoomMove) bool {
	r.stateMu.Lock()

	// don't allow engine dispatched moves not for this game
	if move.GameID != "" && move.GameID != r.game.ID {
		r.stateMu.Unlock()
		return false
	}

	mov := r.legalMoveLocked(move.Move)
	if mov == nil {
		// no move or illegal move provided: capture what we need for messaging
		// before releasing the lock, then resync the player / log the engine
		ofen := r.game.OFEN()
		r.stateMu.Unlock()

		if move.Ctx.IsHuman() {
			// return the human to the authoritative current position
			channel.Unicast(r.CurrentGameStateMessage(false, false), move.Ctx)
		} else {
			// engine gave bad move, major issue
			// TODO handle this somehow if we ever see it
			util.Error(str.CRoom, "engine provided bad move ofen=%s move=%s", ofen, move.Move.UOI)
		}
		return false
	}

	// make move
	if errMove := r.game.Move(mov); errMove != nil {
		r.stateMu.Unlock()
		// bad if this happens
		util.Error(str.CRoom, "bad move given err=%s", errMove.Error())
		return false
	}

	// flip the game clock. This blocks briefly on the clock acknowledgement,
	// but the clock has its own mutex and never calls back into the room, so
	// holding stateMu here cannot deadlock.
	r.flipClock()

	r.game.ToMove = r.game.Position().Turn()

	// capture everything we need after the lock is released so the broadcast
	// and engine request below never touch the game without synchronization
	moveStr := mov.String()
	ofen := r.game.OFEN()
	requestEngine := !move.Ctx.IsBot && r.players.HasBot() &&
		r.game.Outcome() == octad.NoOutcome

	r.stateMu.Unlock()

	// publish move to broadcast channel
	go gamePub.Publish(moveStr, ofen)

	// submit request for engine move after human move
	// only if other player is configured as a bot and game is still ongoing
	if requestEngine {
		r.requestEngineMove()
	}

	// broadcast move to everyone and send the same snapshot back to the player
	stateMsg := r.CurrentGameStateMessage(true, false)
	channel.BroadcastEx(stateMsg, move.Ctx)
	if move.Ctx.IsHuman() {
		channel.Unicast(stateMsg, move.Ctx)
	}

	return true
}

// requestEngineMove requests an engine move for the current position. It is
// called outside any stateMu critical section (by makeMove after it unlocks and
// by handleGameReady), so it locks to read the game fields it needs.
func (r *Instance) requestEngineMove() {
	r.stateMu.Lock()
	req := dispatch.EngineRequest{
		Ctx: channel.SocketContext{
			Channel: r.ID,
			MT:      1,
		},
		ResponseChannel: r.moveChannel,
		Done:            r.done,
		// tag the request with the current game ID so a late-returning search
		// from a previous game (e.g. after a rematch) is dropped by the
		// staleness guard in makeMove instead of being applied to a new game
		GameID: r.game.ID,
		OFEN:   r.game.OFEN(),
		Depth:  r.calcDepthLocked(r.game.ToMove),
	}
	r.stateMu.Unlock()

	go dispatch.SubmitEngine(req)
}

// legalMoveLocked checks to see if the given move is legal and returns its
// corresponding octad move, or nil if invalid. The caller must hold stateMu
// (ValidMoves reads — and lazily caches into — the game).
func (r *Instance) legalMoveLocked(move proto.MovePayload) *octad.Move {
	for _, mov := range r.game.ValidMoves() {
		if mov.String() == move.UOI {
			return mov
		}
	}

	return nil
}

// flipClock flips the internal game clock after a move is made
// and waits for acknowledgement
func (r *Instance) flipClock() {
	// don't flip a clock that has already stopped on a flag: its command and
	// ack channels are closed, so sending would panic. The flagged state is
	// handled separately via the clock StateChannel.
	if r.game.Clock.State(true).Victor != clock.NoVictor {
		return
	}
	ackChannel := r.game.Clock.GetAck()
	// handle clock flipping
	r.game.Clock.ControlChannel <- clock.Flip
	// wait for acknowledgement
	<-ackChannel
}

// calcDepthLocked returns the depth the engine should search to based on the
// remaining time on the clock to try to avoid flagging as much as possible. The
// caller must hold stateMu (it reads the game's variant and clock).
func (r *Instance) calcDepthLocked(color octad.Color) int {
	// depth 7 is about the best we can do in a reasonable timeframe
	// on a good CPU, but it won't work well for bullet
	var depth int

	switch tc := r.game.Variant.Control.Time.Centi(); {
	case tc >= 6000:
		depth = 7
	case tc >= 3000:
		depth = 6
	case tc >= 1500:
		depth = 5
	case tc >= 5:
		depth = 4
	default:
		depth = 4
	}

	var remaining int64
	clockState := r.game.Clock.State(true)
	if color == octad.White {
		remaining = clockState.WhiteTime.Centi()
	} else {
		remaining = clockState.BlackTime.Centi()
	}

	modifier := float64(remaining) / float64(r.game.Variant.Control.Time.Centi())
	if modifier > 1.0 {
		modifier = 1.0
	}

	depth = int(float64(depth) * modifier)

	util.DebugFlag("engine", str.CEng, "selected depth %d for game %s (%.2f%%) time remaining",
		depth, r.ID, modifier*100)
	return depth
}

// tryGameOver will emit a game over broadcast, record the game, and return an event
// to transition the state machine to the GameOver state if the game is actually over
func (r *Instance) tryGameOver(meta channel.SocketContext, abandoned bool) (bool, *fsm.EventDesc) {
	r.stateMu.Lock()

	// nothing to do if the game is still in progress
	if r.game.Outcome() == octad.NoOutcome {
		r.stateMu.Unlock()
		return false, nil
	}

	// terminate the clock goroutine now that the game is decided. Stop is
	// idempotent, so this is a no-op if the game ended on a flag (the
	// clock already stopped itself).
	r.game.Clock.Stop(false, true)

	// update the score, build the final broadcasts, compute the transition
	// event, and snapshot the game for archival — all while holding the lock so
	// readers never observe a half-finished game and the archived copy is taken
	// at a consistent point. The score is updated first so the broadcast
	// messages reflect the result of the game that just finished.
	r.updateScoreLocked()
	stateMsg := r.currentGameStateMessageLocked(true, false)
	overMsg := r.gameOverMessageLocked(abandoned)
	event := r.gameOverEventLocked()

	// shallow value-copy of the game taken under the lock. The room routine is
	// the only writer and the game is now terminal, so subsequent storeGame
	// mutations (AddTagPair) only ever grow the copy's slices via append and do
	// not affect the live game that readers may still observe.
	gameCopy := *r.game

	r.stateMu.Unlock()

	// send final game update to prevent further moves, then the game over
	// message — both outside the lock so the broadcast I/O does not gate it
	channel.Broadcast(stateMsg, meta)
	channel.Broadcast(overMsg, meta)

	// archive the game off the hot path
	go storeGame(gameCopy)

	// return isOver=true with the game over event
	return true, event
}

// updateScoreLocked increments score counters for the winner of a game. The
// caller must hold stateMu (it reads the game outcome and mutates players).
func (r *Instance) updateScoreLocked() {
	switch r.game.Outcome() {
	case octad.Draw:
		r.players.ScoreDraw()
	case octad.WhiteWon:
		r.players.ScoreWin(octad.White)
	case octad.BlackWon:
		r.players.ScoreWin(octad.Black)
	}
}

// storeGame puts the game in object storage for archival purposes
// and also tracks it in the game database
func storeGame(g game.OctadGame) {
	// get parts for Result field
	pgn := g.Game.String()
	parts := strings.Split(pgn, " ")

	// get game state message for Reason field
	_, state := genGameOverState(&g)

	// encode PGN tag pairs
	g.Game.AddTagPair("Event", "Lioctad Test Match")
	g.Game.AddTagPair("Site", "https://lioctad.org")
	g.Game.AddTagPair("Date", g.Start.Format("2006.01.02"))
	g.Game.AddTagPair("Variant", g.Variant.Name)
	g.Game.AddTagPair("Group", string(g.Variant.Group))
	g.Game.AddTagPair("White", g.White)
	g.Game.AddTagPair("Black", g.Black)
	g.Game.AddTagPair("Result", parts[len(parts)-1])
	g.Game.AddTagPair("Reason", state)
	g.Game.AddTagPair("Time", g.Start.Format("15:04:05"))

	pgn = g.Game.String()

	util.DebugFlag("pgn", "PGN", "%s", pgn)

	// year/month/day/HH:MM:SSTZ-(inserted-time-unix).pgn
	key := fmt.Sprintf("%s/%s/%s/%s-%d.pgn",
		g.Start.Format("2006"),
		g.Start.Format("01"),
		g.Start.Format("02"),
		g.Start.Format("15:04:05Z07:00"),
		time.Now().UnixNano())

	err := store.PGNBucket.PutObject(key, []byte(pgn))
	if err != nil {
		util.Error(str.CHMov, str.ERecord, err.Error())
	}
}

// GameState returns the outcome of the current game, or NoOutcome if still in progress
func (r *Instance) GameState() octad.Outcome {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.game.Outcome()
}

// GenTemplatePayload generates a RoomTemplatePayload for the given player by id.
// Called from the HTTP room handler, so it reads players and the game variant
// under stateMu.
func (r *Instance) GenTemplatePayload(id string) message.RoomTemplatePayload {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	_, playerColor := r.players.Lookup(id)

	opponentIsBot := false
	if opp := r.players[playerColor.Other()]; opp != nil {
		opponentIsBot = opp.IsBot
	}

	return message.RoomTemplatePayload{
		RoomID:        r.ID,
		PlayerColor:   playerColor.String(),
		OpponentColor: playerColor.Other().String(),
		OpponentIsBot: opponentIsBot,
		VariantName:   r.game.Variant.Name + " " + string(r.game.Variant.Group),
		Variant:       r.game.Variant,
	}
}

// gameOverEventLocked maps the terminal game outcome to the FSM event that
// transitions the room out of the ongoing state. The caller must hold stateMu
// (it reads the game outcome/method/clock).
func (r *Instance) gameOverEventLocked() *fsm.EventDesc {
	switch r.game.Outcome() {
	case octad.NoOutcome:
		return nil
	case octad.Draw:
		return r.genDrawEventLocked()
	case octad.WhiteWon:
		return r.genWhiteWinEventLocked()
	case octad.BlackWon:
		return r.genBlackWinEventLocked()
	default:
		// this should be impossible
		panic(fmt.Sprintf("Invalid game outcome: %s", r.game.Outcome()))
	}
}

func (r *Instance) genDrawEventLocked() *fsm.EventDesc {
	switch r.game.Method() {
	case octad.InsufficientMaterial:
		return &EventDrawInsufficient
	case octad.Stalemate:
		return &EventDrawStalemate
	case octad.DrawOffer:
		return &EventDrawAgreed
	case octad.ThreefoldRepetition:
		return &EventDrawRepetition
	case octad.TwentyFiveMoveRule:
		return &EventDraw25MoveRule
	default:
		// this should be impossible
		panic(fmt.Sprintf("Invalid white win event: %s", r.game.Method()))
	}
}

func (r *Instance) genWhiteWinEventLocked() *fsm.EventDesc {
	if r.game.Clock.State(true).Victor == clock.White {
		return &EventWhiteWinsTimeout
	}

	switch r.game.Method() {
	case octad.Checkmate:
		return &EventWhiteWinsCheckmate
	case octad.Resignation:
		return &EventWhiteWinsResignation
	default:
		// this should be impossible
		panic(fmt.Sprintf("Invalid white win event: %s", r.game.Method()))
	}
}

func (r *Instance) genBlackWinEventLocked() *fsm.EventDesc {
	if r.game.Clock.State(true).Victor == clock.Black {
		return &EventBlackWinsTimeout
	}

	switch r.game.Method() {
	case octad.Checkmate:
		return &EventBlackWinsCheckmate
	case octad.Resignation:
		return &EventBlackWinsResignation
	default:
		// this should be impossible
		panic(fmt.Sprintf("Invalid white win event: %s", r.game.Method()))
	}
}

// gameOverStateLocked returns the game over state id and status string. The
// caller must hold stateMu (it reads the game).
func (r *Instance) gameOverStateLocked() (int, string) {
	return genGameOverState(r.game)
}

func genGameOverState(g *game.OctadGame) (int, string) {
	switch g.Game.Outcome() {
	case octad.NoOutcome:
		return 0, "FREE, ONLINE OCTAD COMING SOON!"
	case octad.Draw:
		return genDrawState(g)
	case octad.WhiteWon:
		return genWhiteWinState(g)
	default:
		return genBlackWinState(g)
	}
}

func genDrawState(g *game.OctadGame) (int, string) {
	switch g.Game.Method() {
	case octad.InsufficientMaterial:
		return 3, "DRAWN DUE TO INSUFFICIENT MATERIAL"
	case octad.Stalemate:
		return 4, "DRAWN VIA STALEMATE"
	case octad.DrawOffer:
		return 5, "DRAWN BY AGREEMENT"
	case octad.ThreefoldRepetition:
		return 6, "DRAWN BY REPETITION"
	case octad.TwentyFiveMoveRule:
		return 11, "DRAWN DUE TO 25 MOVE RULE"
	default:
		return -1, ""
	}
}

func genWhiteWinState(g *game.OctadGame) (int, string) {
	if g.Clock.State(true).Victor == clock.White {
		return 1, "BLACK OUT OF TIME - WHITE WINS"
	}

	switch g.Game.Method() {
	case octad.Checkmate:
		return 1, "WHITE WINS BY CHECKMATE"
	case octad.Resignation:
		return 7, "BLACK RESIGNED - WHITE WINS"
	}
	return -1, ""
}

func genBlackWinState(g *game.OctadGame) (int, string) {
	if g.Clock.State(true).Victor == clock.Black {
		return 2, "WHITE OUT OF TIME - BLACK WINS"
	}

	switch g.Game.Method() {
	case octad.Checkmate:
		return 2, "BLACK WINS BY CHECKMATE"
	case octad.Resignation:
		return 8, "WHITE RESIGNED - BLACK WINS"
	}
	return -1, ""
}

// gameOverMessageLocked builds the game over payload. The caller must hold
// stateMu (it reads the game, clock, and players).
func (r *Instance) gameOverMessageLocked(abandoned bool) []byte {
	var id int
	var status string

	if abandoned {
		id = -1
		status = "PLAYER ABANDONED - MATCH OVER"
	} else {
		id, status = r.gameOverStateLocked()
	}

	// a bot game will auto-rematch after a fixed delay; tell the client so it
	// can show a countdown. Human-vs-human games wait for a manual rematch.
	autoRematch := 0
	if !abandoned && r.players.HasBot() {
		autoRematch = int(autoRematchDelay.Seconds())
	}

	gameOver := proto.GameOverPayload{
		Winner:      getWinnerString(id),
		StatusID:    id,
		Status:      status,
		Reason:      r.gameOverReasonLocked(abandoned),
		Clock:       r.currentClockLocked(),
		Score:       r.players.ScoreMap(),
		RoomOver:    abandoned,
		AutoRematch: autoRematch,
	}

	return gameOver.Marshal()
}

// gameOverReasonLocked returns a short, structured method code describing how
// the game ended, for the client to render an outcome message. It mirrors the
// human strings produced by the genGameOverState helpers. The caller must hold
// stateMu (it reads the game and clock).
func (r *Instance) gameOverReasonLocked(abandoned bool) string {
	if abandoned {
		return "abandoned"
	}

	switch r.game.Game.Outcome() {
	case octad.NoOutcome:
		return ""
	case octad.Draw:
		switch r.game.Game.Method() {
		case octad.InsufficientMaterial:
			return "insufficient"
		case octad.Stalemate:
			return "stalemate"
		case octad.DrawOffer:
			return "agreement"
		case octad.ThreefoldRepetition:
			return "repetition"
		case octad.TwentyFiveMoveRule:
			return "moverule"
		}
		return ""
	default: // a decisive result
		// a set clock victor means the loser flagged; this takes precedence
		// over the board method, matching genWhiteWinState / genBlackWinState
		if r.game.Clock.State(true).Victor != clock.NoVictor {
			return "time"
		}
		switch r.game.Game.Method() {
		case octad.Checkmate:
			return "checkmate"
		case octad.Resignation:
			return "resignation"
		}
		return ""
	}
}

func getWinnerString(statusId int) string {
	switch statusId {
	case 1, 7:
		return "w"
	case 2, 8:
		return "b"
	}
	return "d"
}
