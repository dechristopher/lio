package room

import (
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

// rooms is a mapping from room ID to instance
var rooms = make(map[string]*Instance)

// Get room instance by id
func Get(id string) (*Instance, error) {
	instance := rooms[id]
	if instance == nil {
		return nil, ErrNoRoom{ID: id}
	}
	return instance, nil
}

// Count returns the number of active rooms
func Count() int {
	return len(rooms)
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

	for rooms[roomId] != nil {
		roomId = config.GenerateCode(7, config.Base58)
	}

	r := &Instance{
		ID:           roomId,
		creator:      params.Creator,
		stateMachine: newStateMachine(),
		params:       params,

		stateChannel:   make(chan State, 1),
		moveChannel:    make(chan *message.RoomMove),
		controlChannel: make(chan message.RoomControl),

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

	r.populateGameConfig()

	// create a new game instance using the provided game config
	var err error
	r.game, err = game.NewOctadGame(r.params.GameConfig)
	if err != nil {
		return nil, err
	}

	// begin room routine
	err = r.init()
	if err != nil {
		return nil, err
	}

	// store room in rooms map
	// TODO sync.Map
	rooms[r.ID] = r

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

// cleanup finishes, closes, and finalizes the room
func (r *Instance) cleanup() {
	util.DebugFlag("room", str.CRoom, "[%s] cleaning up", r.ID)
	// clean up all existing room channels by type
	for _, channelType := range roomChannelTypes {
		c := fmt.Sprintf("%s%s", channelType, r.ID)
		if _, ok := channel.Map.Load(c); ok {
			channel.Map.GetSockMap(c).Cleanup()
		}
	}
	// delete room instance from rooms map
	delete(rooms, r.ID)
}

// event runs a state machine transition using the given EventDesc and args
func (r *Instance) event(event fsm.EventDesc, args ...interface{}) error {
	err := r.stateMachine.Event(event.Name, args)
	if err != nil {
		return err
	}
	return nil
}

// flipBoard flips the player color
func (r *Instance) flipBoard() {
	// change sides
	r.players.FlipColor()

	r.params.GameConfig.White = ""
	r.params.GameConfig.Black = ""

	// repopulate game config values from parameters
	r.populateGameConfig()
}

// populateGameConfig copies relevant parameter values to the game config parameter
// before generating a new game during init, or when flipping the board
func (r *Instance) populateGameConfig() {
	if r.params.GameConfig.White == "" && r.players[octad.White] != nil {
		r.params.GameConfig.White = r.players[octad.White].ID
	}

	if r.params.GameConfig.Black == "" && r.players[octad.Black] != nil {
		r.params.GameConfig.Black = r.players[octad.Black].ID
	}
}

// IsReady returns true if the room is ready for games to be played
func (r *Instance) IsReady() bool {
	hasTwoPlayers, _ := r.players.HasTwoPlayers()
	return hasTwoPlayers || r.HasBot()
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

// Cancel will cancel the room if not past waiting for players state
func (r *Instance) Cancel() bool {
	// only allow room cancellation in the waiting state
	if r.State() != StateWaitingForPlayers {
		return false
	}

	// set cancelled flag to halt room routine loop
	r.cancelled = true

	// emit control message to signal room routine handler to exit
	r.controlChannel <- message.RoomControl{
		Type: message.Cancel,
		Ctx: channel.SocketContext{
			Channel: r.ID,
			MT:      1,
		},
	}

	return true
}

// CanJoin returns true if the given player by uid can participate in the match
// either as a player or a spectator
func (r *Instance) CanJoin(uid string) (asPlayer, asSpectator bool) {
	hasPlayers, _ := r.players.HasTwoPlayers()

	// force into spectator mode if match has both players
	if hasPlayers && !r.players.IsPlayer(uid) {
		// TODO track spectator somehow
		return false, true
	}

	// player can return if P1, or join as P2
	return true, false
}

// Join attempts to join the room as a human player
func (r *Instance) Join(uid, joinToken string) bool {
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
	return r.players.HasBot()
}

// Game returns the game container instance
func (r *Instance) Game() *game.OctadGame {
	return r.game
}

// SendMove writes a move to the room's moveChannel
// to be consumed by a listening routine
func (r *Instance) SendMove(move *message.RoomMove) {
	// prevent first moves before moves are allowed to be played
	if r.State() != StateWaitingForPlayers {
		r.moveChannel <- move
	}
}

// CurrentGameStateMessage returns the octad position, marshalled as a move payload
func (r *Instance) CurrentGameStateMessage(addLast bool, gameStart bool) []byte {
	curr := proto.MovePayload{
		Clock:     r.currentClock(),
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

	fmt.Printf("Game State %v\n", r.State())

	// set legal moves if we're in GameReady or GameOngoing
	// to prevent first moves before moves are allowed to be played
	if r.State() != StateWaitingForPlayers {
		curr.ValidMoves = r.game.LegalMoves()
	}

	// add last move SAN if enabled
	if addLast {
		curr.SAN = r.getSAN()
	}

	return curr.Marshal()
}

// currentClock returns the current clock state via a ClockPayload
func (r *Instance) currentClock() proto.ClockPayload {
	state := r.game.Clock.State(true)
	return proto.ClockPayload{
		Control: r.game.Variant.Control.Time.Centi(),
		Black:   state.BlackTime.Centi(),
		White:   state.WhiteTime.Centi(),
		Lag:     clock.ToCTime(lag.Move.Get()).Centi(),
	}
}

// getSAN returns the last move in algebraic notation
func (r *Instance) getSAN() string {
	if len(r.game.Positions()) > 1 {
		pos := r.game.Positions()[len(r.game.Positions())-2]
		move := r.game.Moves()[len(r.game.Moves())-1]
		return octad.AlgebraicNotation{}.Encode(pos, move)
	}

	return ""
}

// isTurn returns true to ensure moves are received and processed
// only during the given player's turn
func (r *Instance) isTurn(move *message.RoomMove) bool {
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

// makeMove attempts to make the given move, transition game state, and
// notify all channel connections of the game state
func (r *Instance) makeMove(move *message.RoomMove) bool {
	ok := false

	// don't allow engine dispatched moves not for this game
	if move.GameID != "" && move.GameID != r.game.ID {
		return false
	}

	// make move and flip clock if legal
	if mov := r.legalMove(move.Move); mov != nil {
		// make move
		errMove := r.game.Move(mov)
		if errMove != nil {
			// bad if this happens
			util.Error(str.CRoom, "bad move given err=%s", errMove.Error())
			return false
		}

		// flip game clock
		r.flipClock()

		r.game.ToMove = r.game.Position().Turn()

		ok = true

		// publish move to broadcast channel
		go gamePub.Publish(mov.String(), r.game.OFEN())

		// submit request for engine move after human move
		// only if other player is configured as a bot and game is still ongoing
		if !move.Ctx.IsBot && r.players.HasBot() && r.game.Outcome() == octad.NoOutcome {
			r.requestEngineMove()
		}
	}

	// if no move or illegal move provided, return to
	// current position and wait for another move
	if !ok {
		if move.Ctx.IsHuman() {
			channel.Unicast(r.CurrentGameStateMessage(false, false), move.Ctx)
		} else {
			// engine gave bad move, major issue
			// TODO handle this somehow if we ever see it
			util.Error(str.CRoom, "engine provided bad move ofen=%s move=%s", r.game.OFEN(), move.Move.UOI)
		}
		return false
	}

	// broadcast move to everyone and send response back to player
	channel.BroadcastEx(r.CurrentGameStateMessage(true, false), move.Ctx)
	if move.Ctx.IsHuman() {
		channel.Unicast(r.CurrentGameStateMessage(true, false), move.Ctx)
	}

	return true
}

// requestEngineMove requests an engine move based on the given move message
func (r *Instance) requestEngineMove() {
	go dispatch.SubmitEngine(dispatch.EngineRequest{
		Ctx: channel.SocketContext{
			Channel: r.ID,
			MT:      1,
		},
		ResponseChannel: r.moveChannel,
		OFEN:            r.game.OFEN(),
		Depth:           r.calcDepth(r.game.ToMove),
	})
}

// legalMove checks to see if the given move is legal and returns
// its corresponding octad move, or nil if invalid
func (r *Instance) legalMove(move proto.MovePayload) *octad.Move {
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
	ackChannel := r.game.Clock.GetAck()
	// handle clock flipping
	r.game.Clock.ControlChannel <- clock.Flip
	// wait for acknowledgement
	<-ackChannel
}

// calcDepth returns the depth the engine should search to
// based on the remaining time on the clock to try to avoid
// flagging as much as possible
func (r *Instance) calcDepth(color octad.Color) int {
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
	// restart game if over
	if r.game.Outcome() != octad.NoOutcome {
		// send final game update to prevent further moves
		channel.Broadcast(r.CurrentGameStateMessage(true, false), meta)
		// broadcast game over message immediately
		channel.Broadcast(r.gameOverMessage(abandoned), meta)

		// keep track of match score
		r.updateScore()

		// record game result
		wg := &sync.WaitGroup{}
		wg.Add(1)

		go r.recordGame(wg)
		// wait for game copy to be made
		wg.Wait()

		// return isOver=true with game over event
		return true, r.gameOverEvent()
	}

	return false, nil
}

// updateScore will increment score counters for the winner of a game
func (r *Instance) updateScore() {
	switch r.game.Outcome() {
	case octad.Draw:
		r.players.ScoreDraw()
	case octad.WhiteWon:
		r.players.ScoreWin(octad.White)
	case octad.BlackWon:
		r.players.ScoreWin(octad.Black)
	}
}

// recordGame and notify the caller after game is copied
func (r *Instance) recordGame(wg *sync.WaitGroup) {
	// make a copy of game state, so we don't block while storing game
	gameCopy := *r.game
	wg.Done()
	go storeGame(gameCopy)
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

	util.DebugFlag("pgn", "PGN", pgn)

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
	return r.game.Outcome()
}

// GenTemplatePayload generates a RoomTemplatePayload for the given player by id
func (r *Instance) GenTemplatePayload(id string) message.RoomTemplatePayload {
	_, playerColor := r.players.Lookup(id)

	return message.RoomTemplatePayload{
		RoomID:        r.ID,
		PlayerColor:   playerColor.String(),
		OpponentColor: playerColor.Other().String(),
		VariantName:   r.game.Variant.Name + " " + string(r.game.Variant.Group),
		Variant:       r.game.Variant,
	}
}

func (r *Instance) gameOverEvent() *fsm.EventDesc {
	switch r.game.Outcome() {
	case octad.NoOutcome:
		return nil
	case octad.Draw:
		return r.genDrawEvent()
	case octad.WhiteWon:
		return r.genWhiteWinEvent()
	case octad.BlackWon:
		return r.genBlackWinEvent()
	default:
		// this should be impossible
		panic(fmt.Sprintf("Invalid game outcome: %s", r.game.Outcome()))
		return nil
	}
}

func (r *Instance) genDrawEvent() *fsm.EventDesc {
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

func (r *Instance) genWhiteWinEvent() *fsm.EventDesc {
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

func (r *Instance) genBlackWinEvent() *fsm.EventDesc {
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

// gameOverState returns the game over state, or NoOutcome if still in progress
func (r *Instance) gameOverState() (int, string) {
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

func (r *Instance) gameOverMessage(abandoned bool) []byte {
	var id int
	var status string

	if abandoned {
		id = -1
		status = "PLAYER ABANDONED - MATCH OVER"
	} else {
		id, status = r.gameOverState()
	}

	gameOver := proto.GameOverPayload{
		Winner:   getWinnerString(id),
		StatusID: id,
		Status:   status,
		Clock:    r.currentClock(),
		Score:    r.players.ScoreMap(),
		RoomOver: abandoned,
	}

	return gameOver.Marshal()
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
