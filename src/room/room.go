package room

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/dechristopher/octad"
	"github.com/looplab/fsm"

	"github.com/dechristopher/lioctad/bus"
	"github.com/dechristopher/lioctad/channel"
	"github.com/dechristopher/lioctad/channel/handlers"
	"github.com/dechristopher/lioctad/clock"
	"github.com/dechristopher/lioctad/config"
	"github.com/dechristopher/lioctad/dispatch"
	"github.com/dechristopher/lioctad/game"
	"github.com/dechristopher/lioctad/message"
	"github.com/dechristopher/lioctad/store"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

var gamePub = bus.NewPublisher("game", game.Channel)

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
	ID           string
	stateMachine *fsm.FSM
	params       Params

	game *game.OctadGame

	stateChannel   chan State
	moveChannel    chan *message.RoomMove
	controlChannel chan message.RoomControl

	Player1, Player2 string
	P1Color          octad.Color
	P1Bot, P2Bot     bool
	P1Score, P2Score float64

	P1Rematch, P2Rematch bool
}

// Params for room Instance creation
type Params struct {
	Player1, Player2 string
	P1Color          octad.Color
	P1Bot, P2Bot     bool

	GameConfig game.OctadGameConfig
}

// Create a room instance from the given parameters
func Create(params Params) (*Instance, error) {
	// P1 must be a human for P2 to be a human
	if !params.P2Bot && params.P1Bot {
		return nil, ErrBadParamsBots{}
	}

	// P1 must be a human for P2 to be a bot
	// TODO no support for two bots at the moment
	// (internal engine vs internal engine)
	if params.P1Bot && params.P2Bot {
		return nil, ErrBadParamsTwoBots{}
	}

	r := &Instance{
		ID:           config.GenerateCode(7, true),
		stateMachine: newStateMachine(),
		params:       params,

		stateChannel:   make(chan State, 1),
		moveChannel:    make(chan *message.RoomMove),
		controlChannel: make(chan message.RoomControl),

		Player1: params.Player1,
		Player2: params.Player2,
		P1Color: params.P1Color,
		P1Bot:   params.P1Bot,
		P2Bot:   params.P2Bot,
	}

	// Keep track of all channels for off-rpc broadcasts
	// Create a new SockMap and track it under the channel key
	if _, ok := channel.Map[r.ID]; !ok {
		channel.Map[r.ID] = channel.NewSockMap(r.ID)
		go handlers.HandleCrowd(r.ID)
	}

	r.populateGameConfig()

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

	rooms[r.ID] = r

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
	// defer room cleanup, still runs in case of a panic, thanks go
	defer r.cleanup()

	for {
		util.DebugFlag("room", str.CRoom, "[%s] room state transition -> %s", r.ID, r.State())
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
			// TODO redirect / notify players, etc.
			r.handleRoomOver()
			return
		default:
			fmt.Println("sleeping")
			time.Sleep(time.Second * 2)
		}
	}
}

// cleanup finishes, closes, and finalizes the room
func (r *Instance) cleanup() {
	util.DebugFlag("room", str.CRoom, "cleaning up room %s", r.ID)
	channel.Map[r.ID].Cleanup()
	// delete room instance from rooms map
	delete(rooms, r.ID)
}

// event runs a state machine transition using the given EventDesc and args
func (r *Instance) event(event fsm.EventDesc, args ...interface{}) error {
	err := r.stateMachine.Event(event.Name, args)
	if err != nil {
		return err
	}

	// TODO determine the need for the stateChannel
	//r.stateChannel <- r.State()
	return nil
}

// flipBoard flips the player color and returns P1's color
func (r *Instance) flipBoard() octad.Color {
	// change sides
	r.P1Color = r.P1Color.Other()

	r.params.GameConfig.White = ""
	r.params.GameConfig.Black = ""

	// repopulate game config values from parameters
	r.populateGameConfig()
	return r.P1Color
}

// populateGameConfig copies relevant parameter values to the game config parameter
// before generating a new game during init, or when flipping the board
func (r *Instance) populateGameConfig() {
	if r.params.GameConfig.White == "" {
		if r.P1Color == octad.White {
			r.params.GameConfig.White = r.Player1
		} else {
			r.params.GameConfig.White = r.Player2
		}
	}

	if r.params.GameConfig.Black == "" {
		if r.P1Color == octad.White {
			r.params.GameConfig.Black = r.Player2
		} else {
			r.params.GameConfig.Black = r.Player1
		}
	}
}

// Join the room as the second player
func (r *Instance) Join(bid string) (bool, bool) {
	// if room established with both players
	if r.Player1 != "" && r.Player2 != "" {
		// if player returning, allow back
		if r.Player1 == bid || r.Player2 == bid {
			return true, false
		}

		// otherwise, force into spectator mode
		return false, true
	}

	// if player2 joining
	if r.Player1 != bid {
		r.Player2 = bid
		r.Game().Black = bid
		return true, false
	}

	// allow P1 back in before P2 joins
	return r.Player1 == bid, false
}

// State returns the current room state
func (r *Instance) State() State {
	return State(r.stateMachine.Current())
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

// CurrentGameStateMessage octad position, formatted as a move payload
func (r *Instance) CurrentGameStateMessage(addLast bool, gameStart bool) []byte {
	curr := proto.MovePayload{
		Clock:   r.currentClock(),
		OFEN:    r.game.Game.OFEN(),
		MoveNum: len(r.game.Game.Moves()) / 2,
		Check:   r.game.Game.Position().InCheck(),
		Moves:   r.game.MoveHistory(),
		// TODO calculate move processing latency (EWMA)
		Latency:   0,
		GameStart: gameStart,
	}

	// set legal moves if we're in GameReady or GameOngoing
	// to prevent first moves before moves are allowed to be played
	if r.State() != StateWaitingForPlayers {
		curr.ValidMoves = r.game.LegalMoves()
	}

	// set white/black player ids
	if r.P1Color == octad.White {
		curr.White = r.Player1
		curr.Black = r.Player2
	} else {
		curr.White = r.Player2
		curr.Black = r.Player1
	}

	if addLast {
		curr.SAN = r.getSAN()
	}

	return curr.Marshal()
}

// currentClock returns the current clock state via a ClockPayload
func (r *Instance) currentClock() proto.ClockPayload {
	state := r.game.Clock.State()
	return proto.ClockPayload{
		Control: r.game.Variant.Control.Time.Centi(),
		Black:   state.BlackTime.Centi(),
		White:   state.WhiteTime.Centi(),
		Lag:     0,
	}
}

// getSAN returns the last move in algebraic notation
func (r *Instance) getSAN() string {
	if len(r.game.Game.Positions()) > 1 {
		pos := r.game.Game.Positions()[len(r.game.Game.Positions())-2]
		move := r.game.Game.Moves()[len(r.game.Game.Moves())-1]
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
		if r.P2Bot && r.P1Color.Other() == r.game.ToMove {
			return true
		}
	}

	if move.Ctx.BID == r.Player1 {
		if r.P1Color == r.Game().ToMove {
			return true
		}
	} else {
		if r.P1Color.Other() == r.Game().ToMove {
			return true
		}
	}

	return false
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
		errMove := r.game.Game.Move(mov)
		if errMove != nil {
			// bad if this happens
			util.Error(str.CRoom, "bad move given err=%s", errMove.Error())
			return false
		}

		// flip game clock
		r.flipClock(move)

		r.game.ToMove = r.game.Game.Position().Turn()

		ok = true

		// publish move to broadcast channel
		go gamePub.Publish(mov.String(), r.game.Game.OFEN())

		if !move.Ctx.IsBot && r.P2Bot && r.game.Game.Outcome() == octad.NoOutcome {
			// submit request for engine move after human P1 move
			// only if P2 is configured as a bot
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
			util.Error(str.CRoom, "engine provided bad move ofen=%s move=%s", r.game.Game.OFEN(), move.Move.UOI)
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
		OFEN:            r.game.Game.OFEN(),
		Depth:           r.calcDepth(r.game.ToMove),
	})
}

// legalMove checks to see if the given move is legal and returns
// its corresponding octad move, or nil if invalid
func (r *Instance) legalMove(move proto.MovePayload) *octad.Move {
	for _, mov := range r.game.Game.ValidMoves() {
		if mov.String() == move.UOI {
			return mov
		}
	}

	return nil
}

// flipClock flips the internal game clock after a move is made
// and waits for acknowledgement
func (r *Instance) flipClock(move *message.RoomMove) {
	util.DebugFlag("clock", str.CClk, "PRE-FLIP")
	// handle clock flipping
	go func() { r.game.Clock.ControlChannel <- clock.Flip }()

	if move.Ctx.IsBot {
		if r.P2Bot {
			if r.P1Color.Other() == octad.White {
				<-r.game.Clock.WhiteAck
			} else {
				<-r.game.Clock.BlackAck
			}
		}
	} else {
		if move.Ctx.BID == r.Game().White {
			<-r.game.Clock.WhiteAck
		} else {
			<-r.game.Clock.BlackAck
		}
	}
	util.DebugFlag("clock", str.CClk, "POST-FLIP")
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
		depth = 8
	case tc >= 3000:
		depth = 7
	case tc >= 1500:
		depth = 6
	case tc >= 5:
		depth = 5
	default:
		depth = 4
	}

	var remaining int64
	if color == octad.White {
		remaining = r.game.Clock.State().WhiteTime.Centi()
	} else {
		remaining = r.game.Clock.State().BlackTime.Centi()
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
func (r *Instance) tryGameOver(meta channel.SocketContext) (bool, *fsm.EventDesc) {
	// restart game if over
	if r.game.Game.Outcome() != octad.NoOutcome {
		// send final game update to prevent further moves
		channel.Broadcast(r.CurrentGameStateMessage(true, false), meta)
		// broadcast game over message immediately
		channel.Broadcast(r.gameOverMessage(), meta)

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
	switch r.game.Game.Outcome() {
	case octad.Draw:
		r.P1Score += 0.5
	case octad.WhiteWon:
		if r.P1Color == octad.White {
			r.P1Score++
		} else {
			r.P2Score++
		}
	case octad.BlackWon:
		if r.P1Color == octad.Black {
			r.P1Score++
		} else {
			r.P2Score++
		}
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

	err := store.PutObject(store.PGNBucket, key, []byte(pgn))
	if err != nil {
		util.Error(str.CHMov, str.ERecord, err.Error())
	}
}

// GameState returns the outcome of the current game, or NoOutcome if still in progress
func (r *Instance) GameState() octad.Outcome {
	return r.game.Game.Outcome()
}

func (r *Instance) gameOverEvent() *fsm.EventDesc {
	switch r.game.Game.Outcome() {
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
		panic(fmt.Sprintf("Invalid game outcome: %s", r.game.Game.Outcome()))
		return nil
	}
}

func (r *Instance) genDrawEvent() *fsm.EventDesc {
	switch r.game.Game.Method() {
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
		panic(fmt.Sprintf("Invalid white win event: %s", r.game.Game.Method()))
	}
}

func (r *Instance) genWhiteWinEvent() *fsm.EventDesc {
	if r.game.Clock.State().Victor == clock.White {
		return &EventWhiteWinsTimeout
	}

	switch r.game.Game.Method() {
	case octad.Checkmate:
		return &EventWhiteWinsCheckmate
	case octad.Resignation:
		return &EventWhiteWinsResignation
	default:
		// this should be impossible
		panic(fmt.Sprintf("Invalid white win event: %s", r.game.Game.Method()))
	}
}

func (r *Instance) genBlackWinEvent() *fsm.EventDesc {
	if r.game.Clock.State().Victor == clock.Black {
		return &EventBlackWinsTimeout
	}

	switch r.game.Game.Method() {
	case octad.Checkmate:
		return &EventBlackWinsCheckmate
	case octad.Resignation:
		return &EventBlackWinsResignation
	default:
		// this should be impossible
		panic(fmt.Sprintf("Invalid white win event: %s", r.game.Game.Method()))
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
		return 3, "DRAWN DUE TO INSUFFICIENT MATERIAL."
	case octad.Stalemate:
		return 4, "DRAWN BY STALEMATE."
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
	if g.Clock.State().Victor == clock.White {
		return 1, "WHITE WINS ON TIME"
	}

	switch g.Game.Method() {
	case octad.Checkmate:
		return 1, "WHITE WINS BY CHECKMATE"
	case octad.Resignation:
		return 7, "BLACK RESIGNED, WHITE IS VICTORIOUS"
	}
	return -1, ""
}

func genBlackWinState(g *game.OctadGame) (int, string) {
	if g.Clock.State().Victor == clock.Black {
		return 2, "BLACK WINS ON TIME"
	}

	switch g.Game.Method() {
	case octad.Checkmate:
		return 2, "BLACK WINS BY CHECKMATE"
	case octad.Resignation:
		return 8, "WHITE RESIGNED, BLACK IS VICTORIOUS"
	}
	return -1, ""
}

func (r *Instance) gameOverMessage() []byte {
	id, status := r.gameOverState()
	gameOver := proto.GameOverPayload{
		Winner:   getWinnerString(id),
		StatusID: id,
		Status:   status,
		Clock:    r.currentClock(),
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
