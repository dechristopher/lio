package room

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dechristopher/octad/v2"
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
	"github.com/dechristopher/lio/tv"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/variant"
	"github.com/dechristopher/lio/www/ws/proto"
)

var gamePub = bus.NewPublisher("game", game.Channel)

// deployChannelBuffer sizes the blind-deploy submission channel. Two human
// players each submit exactly one arrangement per deploy phase, so two slots
// hold the whole legitimate load without an unbuffered rendezvous. Kept as a
// named constant so the test helpers that rebuild the channel stay in sync with
// production. See SubmitDeploy and arch/DEPLOY_REMATCH_RACES.md (race #2).
const deployChannelBuffer = 2

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

	stateChannel    chan State
	moveChannel     chan *message.RoomMove
	controlChannel  chan message.RoomControl
	deployChannel   chan *message.RoomDeploy
	drawEvalChannel chan *message.RoomDrawEval

	// done is closed exactly once by cleanup when the room routine exits.
	// Senders into the room's channels select on it so they can never block
	// forever once the room is being torn down.
	done chan struct{}

	// stateMu guards the mutable game-state fields that are touched by more
	// than one goroutine: game (both the pointer, which is swapped on rematch,
	// and the octad.Game it points at), players (populated by Join from an HTTP
	// goroutine), rematch (recorded in the game-over handler), and humanMoved
	// (set by the room routine in makeMove). The room routine is the sole writer
	// of game contents, but HTTP/WS handler
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

	// draw records in-game draw-offer agreement between the two seats: a side is
	// marked when it offers (or accepts) a draw, and the game is drawn by
	// agreement once both sides are marked. drawOffer names the color with a
	// currently standing offer (NoColor when none), so a re-offer is idempotent
	// and the opponent's "accept draw" affordance can be surfaced. Both are reset
	// when a move supersedes the offer (makeMove), on a decline, and on a new
	// game. Guarded by stateMu (touched by the room routine and read for
	// broadcasts). See room/controls.go.
	draw      player.Agreement
	drawOffer octad.Color

	// humanMoved reports whether the human player has made at least one move in
	// the current game. It is reset to false when a new game begins (Create's
	// fresh game starts false; a rematch resets it alongside the game swap) and
	// set true on the first human move. It is the engagement signal that
	// distinguishes a player who is actually playing from one whose socket is
	// merely still connected (an idle/backgrounded tab, or someone who wandered
	// off to watch the home-page TV): a bot game the human never moved in is
	// abandoned rather than left to flag. Guarded by stateMu (set by the room
	// routine, read by the abandon detection and the game-over handler).
	humanMoved bool

	// deployDeadline is when the current blind deploy window closes. It is set
	// when the deploy phase begins and read by DeployStateMessage so a
	// (re)connecting client gets the correct remaining time. Zero when not in
	// the deploy phase. Guarded by stateMu.
	deployDeadline time.Time

	// rematchDeadline is when the current human-vs-human rematch window closes
	// (retimed when shortened/restored on an opponent leaving/returning). It is
	// set when handleGameOver begins and read by GameOverStateMessage so a
	// (re)connecting client re-enters the result overlay with an accurate
	// remaining countdown instead of resuming a finished game. Zero for bot games,
	// which are not time-boxed (the finished room stays open). Guarded by stateMu.
	rematchDeadline time.Time

	// deployed holds each side's committed blind arrangement during the deploy
	// phase, so a (re)connecting client can be told its own confirmed order (and
	// both sides' locked-in status) via DeployStateMessage. Written by the deploy
	// handler as submissions arrive and reset when a new deploy phase begins.
	// Guarded by stateMu.
	deployed map[octad.Color]Deployment

	joinToken   string // token to control joining challenges
	cancelToken string // token to control cancelling challenges

	abandoned bool
	cancelled bool

	// public reports whether an open human challenge should be listed in the
	// home-page Open Challenges feed. Challenges default to private (link-only)
	// and the creator opts in to public listing at creation time. Set once in
	// Create and never mutated, so it needs no lock.
	public bool
}

// Params for room Instance creation
type Params struct {
	Creator    string
	Players    player.Players
	GameConfig game.OctadGameConfig
	// Public lists an open human challenge in the home-page Open Challenges
	// feed. Defaults to false (private, link-only); the creator opts in.
	Public bool
	// Deploy enables the blind deploy pre-game: before each game both players
	// privately arrange their home rank, then normal play begins from the
	// assembled position. Defaults to false (classic immediate start).
	Deploy bool
	// BotTimeReserve is the bot difficulty knob: the fraction of the initial
	// clock the bot tries not to dip below when budgeting its searches. A low
	// reserve lets the bot spend nearly its whole clock thinking (hardest); a
	// high reserve makes it move quickly and search shallower (easier). Zero
	// means "unset" and falls back to DefaultBotTimeReserve; use a small
	// positive value for the hardest setting.
	BotTimeReserve float64
	// BotRandomDeploy makes the bot deploy a uniformly random home-rank
	// arrangement instead of one scored by the engine — the easy-difficulty
	// deploy (about a third of arrangements are materially inferior).
	// Defaults to false (engine-scored deploy).
	BotRandomDeploy bool
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
		// the blind deploy pre-game is a property of the chosen variant
		Deploy: variant.Deploy,
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
		// players) can never drop a non-blocking send; each rematch button
		// disables after one click, so no single client floods it
		controlChannel: make(chan message.RoomControl, 2),

		// buffered for 2 so each of the two human players' single blind-deploy
		// submissions is always accepted without the WS read-loop goroutine
		// blocking on an unbuffered send. A bot never uses this channel (it
		// deploys in-handler) and each client sends at most one submission per
		// phase (the client's deployConfirmed flag guards re-sends), so two slots
		// exactly fit the worst legitimate case; SubmitDeploy's non-blocking send
		// then makes an out-of-window straggler impossible to wedge on. See
		// arch/DEPLOY_REMATCH_RACES.md (race #2).
		deployChannel: make(chan *message.RoomDeploy, deployChannelBuffer),

		// buffered for 1 so the engine dispatcher's draw-verdict send (bot games)
		// never blocks the worker even if the game already ended before the verdict
		// returned; a stale verdict is dropped by the game-ongoing handler.
		drawEvalChannel: make(chan *message.RoomDrawEval, 1),

		done: make(chan struct{}),

		players:   params.Players,
		rematch:   player.Agreement{},
		draw:      player.Agreement{},
		drawOffer: octad.NoColor,

		public: params.Public,

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
		case StateDeploy:
			r.handleDeploy()
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
	// drop this room from the home-page TV grid, freeing its slot for backfill.
	// A room that ended without a rematch reaches cleanup, so this is what
	// "swaps out" finished, non-rematching games on the home page.
	tv.Publish(tv.Event{Kind: tv.RoomClosed, RoomID: r.ID})
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
	// subsequent games swap sides by default; a variant may lock colors to keep
	// each player on the same side across rematches
	if !r.params.GameConfig.Variant.LockColors {
		r.players.FlipColor()
	}

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
		// set new join token in payload, and report the open seat as the
		// joiner's color (the joiner is not yet in the players map, so
		// GenTemplatePayload's Lookup leaves PlayerColor as NoColor).
		if payload.IsJoining {
			payload.JoinToken = r.NewJoinToken()
			payload.PlayerColor = r.OpenSeatColor().String()
		}
	}
}

// OpenSeatColor returns the color of the still-open seat in a waiting room — the
// color a joiner would take. It is NoColor once both seats are filled (or for a
// bot room). Locks stateMu (reads players); mirrors hasOpenSeat.
func (r *Instance) OpenSeatColor() octad.Color {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	_, missing := r.players.HasTwoPlayers()
	return missing
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

// hasOpenSeat reports whether the room is still an open challenge: one human
// seat filled and the other empty and joinable (no opponent has joined, and no
// bot occupies it). It is the signal that distinguishes a junk challenge — one
// whose creator left before anyone joined or any game began — from a committed
// game mid-handoff from the challenge page to the board. HasTwoPlayers reports a
// bot seat and an already-joined seat alike as not-missing, so this is false for
// bot rooms and the instant a second player joins. Locks stateMu (reads players).
func (r *Instance) hasOpenSeat() bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	hasTwo, missing := r.players.HasTwoPlayers()
	return !hasTwo && missing != octad.NoColor
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

// humanMovedThisGame reports whether the human has made at least one move in the
// current game. It is the engagement check that complements bothPlayersConnected:
// a socket can stay connected (an idle/backgrounded tab) without the player
// actually playing, so presence alone is not enough to tell an engaged player
// from an idle one.
func (r *Instance) humanMovedThisGame() bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	return r.humanMoved
}

// humanIdleEligible reports whether the room is a bot game in which the human
// has not yet moved and it is currently their turn to move — the condition under
// which a connected-but-idle human (who would otherwise let the bot play the
// game out to a flag, then auto-rematch into another idle game) should be
// abandoned. It deliberately returns false while it is the bot's turn, so we
// never abandon a game that is merely waiting on the engine, and false once the
// human has moved, so a genuinely engaged player is never subject to it.
func (r *Instance) humanIdleEligible() bool {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	botColor := r.players.GetBotColor()
	if botColor == octad.NoColor || r.humanMoved {
		return false
	}
	return r.game.ToMove != botColor
}

// SendMove writes a move to the room's moveChannel to be consumed by the
// room routine. moveChannel is only drained while the game is ready or
// ongoing, so the send selects on r.done to guarantee it can never block a
// caller goroutine (a WS read loop) forever once the room is torn down.
//
// The state guard is a positive allowlist: only StateGameReady and
// StateGameOngoing have a moveChannel reader, so a move landing in any other
// state (deploy, game-over/rematch window, waiting) is dropped outright. A
// blocking send there would park the WS read loop — freezing that client's
// subsequent messages (including a rematch click or deploy submission) until
// the room closed.
//
// Two further defenses close the game-end transition sliver (outcome decided
// but the FSM not yet in StateGameOver — the same window as RequestRematch's
// race #3): a move for an already-decided game is dropped, and a client move
// (which carries no GameID) is stamped with the game it was sent for, so a
// send that still manages to park across a rematch boundary is rejected by
// makeMove's staleness guard instead of being applied to the next game.
func (r *Instance) SendMove(move *message.RoomMove) {
	switch r.State() {
	case StateGameReady, StateGameOngoing:
	default:
		util.DebugFlag("room", str.CRoom, "[%s] dropped move %s: room in state %s", r.ID, move.Move.UOI, r.State())
		return
	}

	r.stateMu.Lock()
	decided := r.game.Outcome() != octad.NoOutcome
	if move.GameID == "" {
		move.GameID = r.game.ID
	}
	r.stateMu.Unlock()
	if decided {
		util.DebugFlag("room", str.CRoom, "[%s] dropped move %s: game already decided", r.ID, move.Move.UOI)
		return
	}

	select {
	case r.moveChannel <- move:
	case <-r.done:
		// room is being torn down; drop the move
	}
}

// SubmitDeploy writes a player's blind deploy arrangement to the room's
// deployChannel, consumed by the deploy handler. It is called from the WS read
// loop, which processes that client's messages serially, so it must never block
// that goroutine.
//
// The State()==StateDeploy guard is necessary but not sufficient: handleDeploy
// stops reading deployChannel the instant it has both arrangements and enters
// deployAndStart, yet the room stays in StateDeploy until deployAndStart fires
// EventDeployComplete near its end. A submission landing in that window passes
// the guard but finds no reader. With an unbuffered channel and a plain blocking
// send the WS read loop would then wedge until the room closed (freezing that
// client's moves and pings, and leaking the goroutine) — the deploy analogue of
// the moveChannel wedge in arch/MATCH_ROOM_ARCH_REVIEW.md finding #3.
//
// Two defenses combine so a straggler is dropped cleanly instead: deployChannel
// is buffered to deployChannelBuffer (which always has room for the ≤2
// legitimate in-phase submissions, so a real arrangement is never dropped), and
// the send is non-blocking with a final default. See
// arch/DEPLOY_REMATCH_RACES.md (race #2).
func (r *Instance) SubmitDeploy(deploy *message.RoomDeploy) {
	if r.State() != StateDeploy {
		util.DebugFlag("room", str.CRoom, "[%s] dropped deploy from %s: room not in deploy phase", r.ID, deploy.Player)
		return
	}

	select {
	case r.deployChannel <- deploy:
	case <-r.done:
		// room is being torn down; drop the deploy
	default:
		// buffer full or the deploy phase is ending (deployAndStart no longer
		// reading): the submission is stale, so drop it rather than block the WS
		// read loop. The client resyncs from the reveal / a board query.
		util.DebugFlag("room", str.CRoom, "[%s] dropped deploy from %s: phase ending or buffer full", r.ID, deploy.Player)
	}
}

// RequestRematch enqueues a rematch agreement on behalf of the requesting
// player. It is called from the WS read loop, so it validates that the request
// comes from a seated player while the game is over, and never blocks the
// caller: if the control buffer is full it drops the request (the player can
// click again). The game-over handler applies the agreement (and auto-agrees a
// bot opponent, so a human's single click restarts a bot game).
func (r *Instance) RequestRematch(meta channel.SocketContext) {
	// A rematch is meaningful once the finishing game is decided. That is
	// normally the StateGameOver window — the only window handleGameOver is
	// reading controlChannel — but there is a hazardous sliver just before it:
	// tryGameOver broadcasts the game-over message (which lights up the client's
	// rematch button) a beat before the room routine fires the FSM transition
	// into StateGameOver. An eager click can land in that sliver, while State()
	// still reads StateGameOngoing, and a bare State()==StateGameOver guard would
	// silently drop it. The player sees "Waiting…" forever and the room hangs out
	// the entire rematch window on an agreement it never recorded.
	//
	// Keying off the decided game outcome in addition to the state closes the
	// window: the terminal outcome is already set when any client can see the
	// game-over message, and the controlChannel is buffered, so the early click
	// is held and consumed by handleGameOver the moment it starts. A fresh game
	// (post-rematch) reports NoOutcome, so this never accepts a stray click
	// outside a real game-over. See arch/DEPLOY_REMATCH_RACES.md (race #3).
	if r.State() != StateGameOver && r.GameState() == octad.NoOutcome {
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

// drainControlChannel non-blockingly empties any control messages still buffered
// on controlChannel. It is called from the room routine at a game boundary (a
// rematch reset) to discard controls that belonged to the game just finished —
// e.g. a duplicate rematch click that raced the client's button-disable, or an
// early click accepted by RequestRematch's decided-outcome window. Left in the
// buffer such a message would be read by the *next* game-over as a spurious
// rematch agreement. Only the room routine touches this between games, so a
// non-blocking drain is race-free and can never wait on new traffic.
//
// Cancel controls are only produced in StateWaitingForPlayers (Cancel guards on
// state), never at this boundary, so nothing meaningful is ever discarded here.
// See arch/DEPLOY_REMATCH_RACES.md (race #3).
func (r *Instance) drainControlChannel() {
	for {
		select {
		case <-r.controlChannel:
		default:
			return
		}
	}
}

// drainDeployChannel non-blockingly empties any deploy submissions still
// buffered on deployChannel. It is called from the room routine as a deploy
// phase begins, so a straggler from the *previous* phase — one that landed in
// the buffer after handleDeploy stopped reading (the race #2 window) — cannot
// be consumed as a legitimate submission in the new phase. Left in place it
// would be attributed to the sender's post-flip color, pre-fill the phase with
// last game's arrangement, and (with two stragglers) complete the phase before
// either player actually deployed. Only the room routine touches the channel
// between phases, so the drain is race-free.
func (r *Instance) drainDeployChannel() {
	for {
		select {
		case <-r.deployChannel:
		default:
			return
		}
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

// setRematchDeadline updates the published rematch-window deadline under stateMu
// so GameOverStateMessage can hand a (re)connecting client the correct remaining
// countdown. Called by handleGameOver as the window opens and when it is
// shortened/restored on an opponent leaving/returning.
func (r *Instance) setRematchDeadline(at time.Time) {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()
	r.rematchDeadline = at
}

// GameOverStateMessage returns the game-over payload for a client (re)connecting
// while the room sits in the game-over / rematch window (a refresh, or returning
// to a match after it ended). It lets the client re-enter the result overlay —
// stopping the clocks and offering the rematch — instead of resuming a finished
// game with the board still live. The rematch countdown is retimed to the actual
// remaining window (from rematchDeadline) so the client's countdown is accurate
// rather than restarting from the full window.
//
// Returns nil if the game is not actually over, so the caller falls back to the
// normal board-state message. abandoned is always false here: an abandoned game
// transitions straight to StateRoomOver and never rests in StateGameOver.
func (r *Instance) GameOverStateMessage() []byte {
	r.stateMu.Lock()
	defer r.stateMu.Unlock()

	if r.game.Outcome() == octad.NoOutcome {
		return nil
	}

	// remaining time in the human rematch window, clamped at zero; a lapsed or
	// unset deadline reads as no countdown (the room is about to close or start
	// the next game). handleGameOver keeps rematchDeadline current, shortening it
	// when an opponent leaves. Bot games have no rematch deadline (the room stays
	// open until the player leaves), so they carry no countdown.
	rematchWin := 0
	if !r.players.HasBot() && !r.rematchDeadline.IsZero() {
		rematchWin = int(time.Until(r.rematchDeadline).Round(time.Second).Seconds())
		if rematchWin < 0 {
			rematchWin = 0
		}
	}

	return r.buildGameOverMessageLocked(false, rematchWin)
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
		SANs:      r.game.SANHistory(),
		OFENs:     r.game.OFENHistory(),
		Latency:   clock.ToCTime(0),
		White:     r.players[octad.White].ID,
		Black:     r.players[octad.Black].ID,
		Score:     r.players.ScoreMap(),
		GameStart: gameStart,
		// carry the game identity so a client that missed the single game-start
		// broadcast recognizes the new game from any later snapshot (its
		// gs/ply staleness guards would otherwise drop it forever)
		GameID: r.game.ID,
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

	// record that the human engaged this game (gates idle-abandon). Engine moves
	// don't count — only a real human move signals the player is actually present
	// and playing.
	if move.Ctx.IsHuman() {
		r.humanMoved = true
	}

	// a played move supersedes any standing draw offer (the offer stands only
	// until the next move by either side). Clearing it here means a late accept /
	// a stale bot verdict for the pre-move position is dropped; clients clear the
	// affordance when the move broadcast arrives. See room/controls.go.
	if r.drawOffer != octad.NoColor {
		r.drawOffer = octad.NoColor
		r.draw = player.NewAgreement()
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

	// snapshot the post-move state for the home-page TV stream while still locked
	tvMove := r.tvEventLocked(tv.Move)

	r.stateMu.Unlock()

	// publish move to broadcast channel
	go gamePub.Publish(moveStr, ofen)

	// stream the move to home-page TV viewers
	tv.Publish(tvMove)

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
	depth, budget := r.calcSearchLocked(r.game.ToMove)
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
		Depth:  depth,
		Budget: budget,
	}
	r.stateMu.Unlock()

	go dispatch.SubmitEngine(req)
}

// requestEngineDeploy asks the engine dispatcher to choose the bot's blind
// home-rank arrangement, delivering the result on ch. It mirrors
// requestEngineMove: the selection runs off the deploy handler goroutine so the
// deploy timer and cancellation stay responsive. ch must be buffered by the
// caller so the dispatcher's send never blocks even if the deploy phase has
// already ended (see handleDeploy).
func (r *Instance) requestEngineDeploy(botColor octad.Color, ch chan *message.RoomBotDeploy) {
	go dispatch.SubmitDeploy(dispatch.DeployRequest{
		Color:           botColor,
		Random:          r.params.BotRandomDeploy,
		ResponseChannel: ch,
	})
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

// Bot search time-budget policy: the engine iteratively deepens toward the
// depth ceiling and returns the best move found when the budget expires, so
// the bot's strength degrades gracefully under time pressure instead of the
// search running long and flagging.
const (
	// DefaultBotTimeReserve is the BotTimeReserve used when Params doesn't set
	// one: the bot banks a fifth of its initial clock and paces its thinking
	// with the rest.
	DefaultBotTimeReserve = 0.2
	// botMoveHorizon is the number of future bot moves the spendable time
	// (remaining minus reserve) is spread across when budgeting one search.
	botMoveHorizon = 10
	// botBudgetSafety is clock time the budget never touches, covering the
	// dispatch/socket overhead between the search deadline and the move
	// actually landing, so a full-budget search can't flag the bot.
	botBudgetSafety = time.Second
	// botMinBudget is the per-move budget floor once the bot is at or below
	// its reserve. Shallow depths complete in well under this on a 4x4 board,
	// so the bot still moves nearly instantly when scrambling.
	botMinBudget = 50 * time.Millisecond
)

// calcSearchLocked returns the depth ceiling and time budget for an engine
// search on behalf of color. The depth ceiling comes from the time control;
// the budget comes from the bot's remaining clock: time above the configured
// reserve (the difficulty knob, see Params.BotTimeReserve) is spread across a
// horizon of future moves, plus any per-move increment/delay regain. The
// caller must hold stateMu (it reads the game's variant and clock).
func (r *Instance) calcSearchLocked(color octad.Color) (int, time.Duration) {
	// depth 7 is about the best we can do in a reasonable timeframe
	// on a good CPU, but it won't work well for bullet
	var depth int

	control := r.game.Variant.Control
	switch tc := control.Time.Centi(); {
	case tc >= 6000:
		depth = 7
	case tc >= 3000:
		depth = 6
	case tc >= 1500:
		depth = 5
	default:
		depth = 4
	}

	var remaining time.Duration
	clockState := r.game.Clock.State(true)
	if color == octad.White {
		remaining = time.Duration(clockState.WhiteTime.Centi()) * clock.Centisecond
	} else {
		remaining = time.Duration(clockState.BlackTime.Centi()) * clock.Centisecond
	}

	reserveFraction := r.params.BotTimeReserve
	if reserveFraction <= 0 {
		reserveFraction = DefaultBotTimeReserve
	}
	initial := time.Duration(control.Time.Centi()) * clock.Centisecond
	reserve := time.Duration(float64(initial) * reserveFraction)

	budget := (remaining - reserve) / botMoveHorizon
	if budget < botMinBudget {
		budget = botMinBudget
	}

	// increment/delay time comes back every move, so it is spent on top of
	// the paced budget without ever eating into the reserve
	budget += time.Duration(control.Increment.Centi()+control.Delay.Centi()) * clock.Centisecond

	// never budget into the safety margin, no matter how generous the pacing
	if maxBudget := remaining - botBudgetSafety; budget > maxBudget {
		budget = maxBudget
	}
	if budget < botMinBudget {
		budget = botMinBudget
	}

	util.DebugFlag("engine", str.CEng, "selected depth %d budget %s for game %s (%s remaining)",
		depth, budget, r.ID, remaining)
	return depth, budget
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

	// snapshot the terminal position for the TV stream; the room keeps its grid
	// slot until it actually closes (it may rematch), so this just freezes the
	// shown board on the final position
	tvEnd := r.tvEventLocked(tv.End)

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

	// stream the final position to home-page TV viewers
	tv.Publish(tvEnd)

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

// buildArchivePGN assembles the archival PGN for a finished game: the standard
// tag-pair roster plus, for games that don't begin from the standard octad start
// (deploy-mode games start from a rearranged home rank), a SetUp/FEN tag pair
// carrying the starting OFEN so the game can be replayed from the correct
// initial position for analysis and database import.
func buildArchivePGN(g game.OctadGame) string {
	// the Result token is the last space-delimited field of the movetext
	parts := strings.Split(g.Game.String(), " ")

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

	// record a non-standard starting position (e.g. a deploy-mode game) as a
	// SetUp/FEN tag pair so the movetext replays from the correct initial OFEN.
	// The value is octad's OFEN, but the tag key must be the PGN-standard "FEN":
	// that's the only key octad's own PGN decoder (and database Scanner) reads to
	// seed a custom start — an "OFEN" key is silently ignored and the game would
	// re-import from the standard position. See TestBuildArchivePGNDeployStart.
	if start, serr := octad.StartingPosition(); serr == nil {
		if startOFEN := g.Game.Positions()[0].String(); startOFEN != start.String() {
			g.Game.AddTagPair("SetUp", "1")
			g.Game.AddTagPair("FEN", startOFEN)
		}
	}

	return g.Game.String()
}

// storeGame puts the game in object storage for archival purposes
// and also tracks it in the game database
func storeGame(g game.OctadGame) {
	pgn := buildArchivePGN(g)

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
		Public:        r.public,
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

// gameOverMessageLocked builds the live-finish game over payload. A
// human-vs-human game holds a manual rematch window; send its full length so the
// client can count down to the room closing. Bot games are neither
// auto-rematched nor time-boxed (the finished room stays open for review and
// manual rematch), so they carry no countdown. GameOverStateMessage builds the
// equivalent payload with the remaining window for a (re)connecting client. The
// caller must hold stateMu (it reads the game, clock, and players).
func (r *Instance) gameOverMessageLocked(abandoned bool) []byte {
	rematchWin := 0
	if !abandoned && !r.players.HasBot() {
		rematchWin = int(rematchWindow.Seconds())
	}

	return r.buildGameOverMessageLocked(abandoned, rematchWin)
}

// buildGameOverMessageLocked assembles the game over payload with the human
// rematch-window countdown (rematchWin; zero for bot games, which are neither
// auto-rematched nor time-boxed). Callers set the full window on a live finish
// and the remaining window for a (re)connecting client. The caller must hold
// stateMu (it reads the game, clock, and players).
func (r *Instance) buildGameOverMessageLocked(abandoned bool, rematchWin int) []byte {
	var id int
	var status string

	if abandoned {
		id = -1
		status = "PLAYER ABANDONED - MATCH OVER"
	} else {
		id, status = r.gameOverStateLocked()
	}

	gameOver := proto.GameOverPayload{
		Winner:        getWinnerString(id),
		StatusID:      id,
		Status:        status,
		Reason:        r.gameOverReasonLocked(abandoned),
		Clock:         r.currentClockLocked(),
		Score:         r.players.ScoreMap(),
		RoomOver:      abandoned,
		RematchWindow: rematchWin,
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
