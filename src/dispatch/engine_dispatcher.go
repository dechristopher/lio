package dispatch

import (
	"math"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/engine"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// EngineRequest is a request for engine evaluation
type EngineRequest struct {
	GameID string
	OFEN   string
	// History is the game's position history (OFENs oldest-first, including
	// the current position). The engine uses it to see draws by repetition
	// coming — without it the search reconstructs the game from the bare OFEN
	// and would shuffle won endgames into threefold draws.
	History []string
	Depth   int
	// Budget bounds how long the search may run (0 = unbounded): the engine
	// iteratively deepens toward Depth and returns the best move found when
	// the budget expires, so the bot answers in time instead of flagging.
	Budget time.Duration
	// Persona is the bot's difficulty bundle (room.Params.BotPersona resolved):
	// it caps Depth and applies the persona's imperfect move selection. The
	// zero value plays full-strength (engine.SearchPersona treats it like the
	// Queen), matching pre-persona behavior.
	Persona         engine.Persona
	ResponseChannel chan *message.RoomMove
	// Done, if set, signals that the requesting room has been torn down so
	// the worker can drop its result instead of blocking forever on the
	// unbuffered ResponseChannel.
	Done <-chan struct{}
	Ctx  channel.SocketContext
}

// DrawRequest asks the engine whether the bot should accept a human's draw
// offer in the given position. Like EngineRequest it is served off the room
// goroutine; the verdict is returned on ResponseChannel, which the caller sizes
// (buffered) so the send never blocks even if the game already ended. The
// request is tagged with the game and position it evaluates so a verdict that
// arrives after a move landed is dropped by the room.
type DrawRequest struct {
	GameID string
	OFEN   string
	// History mirrors EngineRequest.History so the draw-verdict search is
	// repetition-aware too.
	History []string
	Depth   int
	// Budget bounds the verdict search like EngineRequest.Budget (0 = unbounded)
	Budget          time.Duration
	ResponseChannel chan *message.RoomDrawEval
	// Done, if set, signals that the requesting room has been torn down so the
	// worker can drop its result instead of blocking on the response channel.
	Done <-chan struct{}
}

// DeployRequest is a request for the engine to choose a bot's blind home-rank
// deployment. Like EngineRequest it is served off the room goroutine; the chosen
// board-order placement is returned on ResponseChannel, which the caller sizes
// (buffered) so this send never blocks even if the deploy phase has already
// ended (see room.handleDeploy).
type DeployRequest struct {
	Color octad.Color
	// Random skips the expected-value scoring and deploys a uniformly random
	// arrangement — the easy-difficulty deploy (see engine.RandomDeployment).
	Random          bool
	ResponseChannel chan *message.RoomBotDeploy
}

// EngineDispatcher is a dispatcher for engine evaluation requests
type EngineDispatcher struct {
	Requests       chan EngineRequest
	DeployRequests chan DeployRequest
	DrawRequests   chan DrawRequest
}

var instance EngineDispatcher

// UpEngine brings the engine dispatcher online
func UpEngine() {
	instance = EngineDispatcher{
		Requests:       make(chan EngineRequest),
		DeployRequests: make(chan DeployRequest),
		DrawRequests:   make(chan DrawRequest),
	}
	go instance.run()

	// pre-compute the blind-deploy candidate lists off the boot path so the
	// first bot deploy doesn't pay the scoring cost. Warming here (engine
	// bring-up) rather than in an engine package init keeps instances that
	// never run the engine free of the overhead.
	go engine.WarmDeployCache()
}

// SubmitEngine submits a request to the engine dispatcher
func SubmitEngine(request EngineRequest) {
	instance.Requests <- request
}

// SubmitDeploy submits a bot deploy-selection request to the engine dispatcher
func SubmitDeploy(request DeployRequest) {
	instance.DeployRequests <- request
}

// SubmitDraw submits a bot draw-offer evaluation request to the engine dispatcher
func SubmitDraw(request DrawRequest) {
	instance.DrawRequests <- request
}

// dispatcher for engine evaluation requests from games against the engine
func (d *EngineDispatcher) run() {
	util.Debug(str.CEng, str.DEngOk)
	for {
		select {
		case request := <-d.Requests:
			go d.worker(request)
		case request := <-d.DeployRequests:
			go d.deployWorker(request)
		case request := <-d.DrawRequests:
			go d.drawWorker(request)
		}
	}
}

// drawWorker evaluates the current position and decides whether the bot accepts
// a draw offer: it agrees only when neither side is winning by more than
// engine.DrawEvalMargin, and otherwise declines and plays on. The verdict is
// returned on the request's response channel, buffered by the caller so this
// send never blocks even if the game already ended and no one is reading.
func (d *EngineDispatcher) drawWorker(r DrawRequest) {
	eval := engine.Search(r.OFEN, r.History, r.Depth, r.Budget, engine.MinimaxAB)
	accept := math.Abs(eval.Eval) < engine.DrawEvalMargin

	util.DebugFlag("dispatch", str.CEng, "[%s] draw eval %.1f -> accept=%t", r.OFEN, eval.Eval, accept)

	response := &message.RoomDrawEval{
		GameID: r.GameID,
		OFEN:   r.OFEN,
		Accept: accept,
	}

	select {
	case r.ResponseChannel <- response:
	case <-r.Done:
		util.DebugFlag("dispatch", str.CEng, "[%s] room gone, dropping draw verdict", r.OFEN)
	}
}

// deployWorker chooses the bot's blind home-rank arrangement via the engine and
// returns it on the request's response channel.
func (d *EngineDispatcher) deployWorker(r DeployRequest) {
	util.DebugFlag("dispatch", str.CEng, "deploy request received for %s (random=%t), selecting..",
		r.Color, r.Random)

	var placement engine.DeployPlacement
	if r.Random {
		placement = engine.RandomDeployment()
	} else {
		placement = engine.SelectDeployment(r.Color)
	}

	util.DebugFlag("dispatch", str.CEng, "deploy selected %s for %s", placement, r.Color)

	// the caller buffers the response channel, so this send never blocks
	// even if the room's deploy phase already ended and no one is reading
	r.ResponseChannel <- &message.RoomBotDeploy{
		Color:     r.Color,
		Placement: placement,
	}
}

// worker to actually crunch, find, and return the engine move
func (d *EngineDispatcher) worker(r EngineRequest) {
	// ensure upstream handlers know this move is from a bot
	r.Ctx.IsBot = true
	r.Ctx.UID = ""

	util.DebugFlag("dispatch", str.CEng, "[%s] request received, searching(%d)..", r.OFEN, r.Depth)

	move := engine.SearchPersona(r.OFEN, r.History, r.Depth, r.Budget, r.Persona)

	util.DebugFlag("dispatch", str.CEng, "[%s] found move %s", r.OFEN, move.Move.String())

	// write response move to given channel, but bail if the room is gone
	response := &message.RoomMove{
		Player: "engine",
		GameID: r.GameID,
		Move: proto.MovePayload{
			Clock: proto.ClockPayload{},
			UOI:   move.Move.String(),
		},
		Ctx: r.Ctx,
	}

	select {
	case r.ResponseChannel <- response:
	case <-r.Done:
		util.DebugFlag("dispatch", str.CEng, "[%s] room gone, dropping engine move %s", r.OFEN, move.Move.String())
	}
}
