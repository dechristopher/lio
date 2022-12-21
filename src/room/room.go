package room

import (
	"fmt"
	"strings"
	"sync"
	"time"

	wsv1 "github.com/dechristopher/lio/proto"
	"google.golang.org/protobuf/proto"

	"github.com/dechristopher/octad"
	"github.com/looplab/fsm"

	"github.com/dechristopher/lio/bus"
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/channel/handlers"
	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/dispatch"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/store"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

var gamePub = bus.NewPublisher("game", game.Channel)

// Type of room channel
type Type string

const (
	room Type = ""
	wait Type = "wait/"
)

var roomChannelTypes = []Type{
	room,
	wait,
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

// GetAll room instances
func GetAll() []*Instance {
	// Convert map to slice of values.
	var values []*Instance
	for _, value := range rooms {
		values = append(values, value)
	}

	return values
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

	stateChannel   chan wsv1.RoomState
	moveChannel    chan *message.RoomMove
	controlChannel chan *message.RoomControl

	players player.Players
	rematch player.Agreement

	joinToken   string // token to control joining challenges
	cancelToken string // token to control cancelling challenges

	abandoned bool
	cancelled bool
}

// Params for room Instance creation1
type Params struct {
	Creator    string
	Players    player.Players
	GameConfig game.OctadGameConfig
}

// NewParams returns a new parameters object configured
// using the given variant
func NewParams(creatorId string, variant *wsv1.Variant) Params {
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

		stateChannel:   make(chan wsv1.RoomState, 1),
		moveChannel:    make(chan *message.RoomMove),
		controlChannel: make(chan *message.RoomControl),

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
		case wsv1.RoomState_ROOM_STATE_WAITING_FOR_PLAYERS:
			r.handleWaitingForPlayers()
		case wsv1.RoomState_ROOM_STATE_GAME_READY:
			r.handleGameReady()
		case wsv1.RoomState_ROOM_STATE_GAME_ONGOING:
			r.handleGameOngoing()
		case wsv1.RoomState_ROOM_STATE_GAME_OVER:
			r.handleGameOver()
		case wsv1.RoomState_ROOM_STATE_ROOM_OVER:
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

// Cancel will cancel the room if not past waiting for players state
func (r *Instance) Cancel() bool {
	// only allow room cancellation in the waiting state
	if r.State() != wsv1.RoomState_ROOM_STATE_WAITING_FOR_PLAYERS {
		return false
	}

	// set cancelled flag to halt room routine loop
	r.cancelled = true

	// emit control message to signal room routine handler to exit
	r.controlChannel <- &message.RoomControl{
		Type: message.Cancel,
		Ctx: channel.SocketContext{
			Channel: r.ID,
			MT:      2,
		},
	}

	return true
}

// CanJoin returns true if the given player by uid can participate in the match
// either as a player or a spectator TODO is this needed?
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
func (r *Instance) Join(uid string) bool {
	// if room established with both players // TODO this could update to check the socket connections
	hasTwo, missingColor := r.players.HasTwoPlayers()

	// if second player joining
	if !hasTwo && missingColor != octad.NoColor {
		// set joining player
		r.players[missingColor] = &player.Player{
			ID: uid,
		}

		// set internal game instance state
		if missingColor == octad.White {
			r.game.White = uid
		} else {
			r.game.Black = uid
		}

		// emit control message to signal room routine handler to update state
		r.controlChannel <- &message.RoomControl{
			Type: message.Join,
			Ctx: channel.SocketContext{
				Channel: r.ID,
				MT:      2,
			},
		}

		// joined properly
		return true
	}

	// game already has both players
	return false
}

// Rematch attempts to accept a players request for a rematch
func (r *Instance) Rematch(uid string) bool {
	if r.State() == wsv1.RoomState_ROOM_STATE_GAME_OVER {
		roomSocketMap := channel.Map.GetSockMap(r.ID)
		blackPlayer := r.players[octad.Black]
		whitePlayer := r.players[octad.White]
		blackPlayerPresent := blackPlayer.IsBot || roomSocketMap.Has(blackPlayer.ID)
		whitePlayerPresent := whitePlayer.IsBot || roomSocketMap.Has(whitePlayer.ID)
		bothPlayersPresent := blackPlayerPresent && whitePlayerPresent

		if bothPlayersPresent {
			// emit control message to signal room routine handler to update state
			r.controlChannel <- &message.RoomControl{
				Type: message.Rematch,
				Ctx: channel.SocketContext{
					Channel: r.ID,
					UID:     uid,
					MT:      2,
				},
			}

			// player's rematch request was accepted
			return true
		}
	}

	// player's rematch request was denied
	return false
}

// NotifyWaiting notifies the waiting player(s) that games have begun
// by sending redirect messages to the room
func (r *Instance) NotifyWaiting() {
	waitingChannelName := fmt.Sprintf("%s%s", wait, r.ID)
	// ensure channel exists before notifying players
	if waitingChannel := channel.Map.GetSockMap(waitingChannelName); waitingChannel != nil {
		meta := channel.SocketContext{
			Channel: waitingChannelName,
			MT:      2,
		}

		websocketMessage := wsv1.WebsocketMessage{Data: &wsv1.WebsocketMessage_RedirectPayload{RedirectPayload: &wsv1.RedirectPayload{
			Location: fmt.Sprintf("/%s", r.ID),
		}}}

		payload, err := proto.Marshal(&websocketMessage)
		if err != nil {
			util.Error(str.CChan, "error encoding redirect message err=%s", err.Error())
		}

		// broadcast redirect message
		channel.Broadcast(payload, meta)
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

// Players returns the players in the room
func (r *Instance) Players() player.Players {
	return r.players
}

// State returns the current room state
func (r *Instance) State() wsv1.RoomState {
	state := wsv1.RoomState_value[r.stateMachine.Current()]
	return wsv1.RoomState(state)
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
	if r.State() != wsv1.RoomState_ROOM_STATE_WAITING_FOR_PLAYERS {
		r.moveChannel <- move
	}
}

// GetSerializedGameState returns the current game state in a marshalled move payload
func (r *Instance) GetSerializedGameState() []byte {
	moveMessage := wsv1.MovePayload{
		RoomState:     r.State(),
		San:           r.getSAN(),
		Ofen:          r.game.OFEN(),
		Clock:         r.currentClock(),
		ValidMoves:    r.game.LegalMoves(),
		Check:         r.game.Position().InCheck(),
		WhitePlayerId: r.players[octad.White].ID,
		BlackPlayerId: r.players[octad.Black].ID,
		Moves: &wsv1.Moves{
			Moves: r.game.MoveHistory(),
		},
		Score: &wsv1.ScorePayload{
			Black: r.players.ScoreMap().Black,
			White: r.players.ScoreMap().White,
		},
	}

	// set legal moves if we're in GameReady or GameOngoing
	// to prevent first moves before moves are allowed to be played
	if r.State() != wsv1.RoomState_ROOM_STATE_WAITING_FOR_PLAYERS {
		moveMessage.ValidMoves = r.game.LegalMoves()
	}

	websocketMessage := wsv1.WebsocketMessage{
		Data: &wsv1.WebsocketMessage_MovePayload{
			MovePayload: &moveMessage,
		},
	}

	payload, err := proto.Marshal(&websocketMessage)
	if err != nil {
		util.Error(str.CRoom, "error encoding move message err=%s", err.Error())
	}

	util.DebugFlag("ws", str.CWS, str.DWSSend, websocketMessage.Data)

	return payload
}

// currentClock returns the current clock state via a ClockPayload
func (r *Instance) currentClock() *wsv1.ClockPayload {
	state := r.game.Clock.State(true)
	return &wsv1.ClockPayload{
		Variant: clock.ConvertVariantTimeControl(r.game.Variant),
		Black:   state.BlackTime.Milliseconds(),
		White:   state.WhiteTime.Milliseconds(),
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
			channel.Unicast(r.GetSerializedGameState(), move.Ctx)
		} else {
			// engine gave bad move, major issue
			// TODO handle this somehow if we ever see it
			util.Error(str.CRoom, "engine provided bad move ofen=%s move=%s", r.game.OFEN(), move.Move.Uoi)
		}
		return false
	}

	// broadcast move to everyone and send response back to player
	channel.BroadcastEx(r.GetSerializedGameState(), move.Ctx)
	if move.Ctx.IsHuman() {
		channel.Unicast(r.GetSerializedGameState(), move.Ctx)
	}

	return true
}

// requestEngineMove requests an engine move based on the given move message
func (r *Instance) requestEngineMove() {
	go dispatch.SubmitEngine(dispatch.EngineRequest{
		Ctx: channel.SocketContext{
			Channel: r.ID,
			MT:      2,
		},
		ResponseChannel: r.moveChannel,
		OFEN:            r.game.OFEN(),
		Depth:           r.calcDepth(r.game.ToMove),
	})
}

// legalMove checks to see if the given move is legal and returns
// its corresponding octad move, or nil if invalid
func (r *Instance) legalMove(move *wsv1.MovePayload) *octad.Move {
	for _, mov := range r.game.ValidMoves() {
		if mov.String() == move.Uoi {
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
	control := time.Duration(r.game.Variant.Control.InitialTime)

	// TODO need to evaluate these values
	switch tc := control; {
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
		remaining = clockState.WhiteTime.Nanoseconds()
	} else {
		remaining = clockState.BlackTime.Nanoseconds()
	}

	modifier := float64(remaining) / float64(control.Nanoseconds())
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
		channel.Broadcast(r.GetSerializedGameState(), meta)
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

// GenStatusPayload generates a RoomStatusPayload
func (r *Instance) GenStatusPayload() message.RoomStatusPayload {
	payload := message.RoomStatusPayload{
		RoomID:    r.ID,
		RoomState: r.State(),
		Variant:   r.Game().Variant,
		Players:   r.Players(),
	}

	return payload
}

// GenLobbyPayload generates a RoomLobbyPayload for the given player by id
func (r *Instance) GenLobbyPayload(uid string) wsv1.RoomPayload {
	isCreator := r.IsCreator(uid)
	playerColor := octad.NoColor

	if isCreator {
		_, playerColor = r.players.Lookup(uid)
	} else {
		_, creatorColor := r.players.Lookup(r.creator)
		playerColor = creatorColor.Other()
	}

	color := wsv1.PlayerColor_PLAYER_COLOR_UNSPECIFIED

	if playerColor == octad.White {
		color = wsv1.PlayerColor_PLAYER_COLOR_WHITE
	} else if playerColor == octad.Black {
		color = wsv1.PlayerColor_PLAYER_COLOR_BLACK
	}

	return wsv1.RoomPayload{
		RoomId:      r.ID,
		RoomState:   r.State(),
		IsCreator:   isCreator,
		PlayerColor: color,
		Variant:     clock.ConvertVariantTimeControl(r.game.Variant),
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
func (r *Instance) gameOverState() (wsv1.GameOutcome, string) {
	return genGameOverState(r.game)
}

func genGameOverState(g *game.OctadGame) (wsv1.GameOutcome, string) {
	switch g.Game.Outcome() {
	case octad.NoOutcome:
		return wsv1.GameOutcome_GAME_OUTCOME_UNSPECIFIED, "FREE, ONLINE OCTAD COMING SOON!"
	case octad.Draw:
		return genDrawState(g)
	case octad.WhiteWon:
		return genWhiteWinState(g)
	default:
		return genBlackWinState(g)
	}
}

func genDrawState(g *game.OctadGame) (wsv1.GameOutcome, string) {
	switch g.Game.Method() {
	case octad.InsufficientMaterial:
		return wsv1.GameOutcome_GAME_OUTCOME_DRAW, "DRAWN DUE TO INSUFFICIENT MATERIAL"
	case octad.Stalemate:
		return wsv1.GameOutcome_GAME_OUTCOME_DRAW, "DRAWN VIA STALEMATE"
	case octad.DrawOffer:
		return wsv1.GameOutcome_GAME_OUTCOME_DRAW, "DRAWN BY AGREEMENT"
	case octad.ThreefoldRepetition:
		return wsv1.GameOutcome_GAME_OUTCOME_DRAW, "DRAWN BY REPETITION"
	case octad.TwentyFiveMoveRule:
		return wsv1.GameOutcome_GAME_OUTCOME_DRAW, "DRAWN DUE TO 25 MOVE RULE"
	default:
		return wsv1.GameOutcome_GAME_OUTCOME_UNSPECIFIED, ""
	}
}

func genWhiteWinState(g *game.OctadGame) (wsv1.GameOutcome, string) {
	if g.Clock.State(true).Victor == clock.White {
		return wsv1.GameOutcome_GAME_OUTCOME_WHITE_WINS, "BLACK OUT OF TIME - WHITE WINS"
	}

	switch g.Game.Method() {
	case octad.Checkmate:
		return wsv1.GameOutcome_GAME_OUTCOME_WHITE_WINS, "WHITE WINS BY CHECKMATE"
	case octad.Resignation:
		return wsv1.GameOutcome_GAME_OUTCOME_WHITE_WINS, "BLACK RESIGNED - WHITE WINS"
	}
	return wsv1.GameOutcome_GAME_OUTCOME_UNSPECIFIED, ""
}

func genBlackWinState(g *game.OctadGame) (wsv1.GameOutcome, string) {
	if g.Clock.State(true).Victor == clock.Black {
		return wsv1.GameOutcome_GAME_OUTCOME_BLACK_WINS, "WHITE OUT OF TIME - BLACK WINS"
	}

	switch g.Game.Method() {
	case octad.Checkmate:
		return wsv1.GameOutcome_GAME_OUTCOME_BLACK_WINS, "BLACK WINS BY CHECKMATE"
	case octad.Resignation:
		return wsv1.GameOutcome_GAME_OUTCOME_BLACK_WINS, "WHITE RESIGNED - BLACK WINS"
	}
	return wsv1.GameOutcome_GAME_OUTCOME_UNSPECIFIED, ""
}

func (r *Instance) gameOverMessage(abandoned bool) []byte {
	var gameOutcome wsv1.GameOutcome
	var outcomeDetails string

	if abandoned {
		gameOutcome = wsv1.GameOutcome_GAME_OUTCOME_UNSPECIFIED
		outcomeDetails = "PLAYER ABANDONED - MATCH OVER"
	} else {
		gameOutcome, outcomeDetails = r.gameOverState()
	}

	websocketMessage := wsv1.WebsocketMessage{
		Data: &wsv1.WebsocketMessage_GameOverPayload{
			GameOverPayload: &wsv1.GameOverPayload{
				RoomOver:       abandoned,
				GameOutcome:    gameOutcome,
				OutcomeDetails: outcomeDetails,
				Score: &wsv1.ScorePayload{
					Black: r.players.ScoreMap().Black,
					White: r.players.ScoreMap().White,
				},
			},
		},
	}

	payload, err := proto.Marshal(&websocketMessage)
	if err != nil {
		util.Error(str.CRoom, "error encoding game over message err=%s", err.Error())
	}

	return payload
}
