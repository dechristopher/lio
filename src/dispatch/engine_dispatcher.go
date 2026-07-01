package dispatch

import (
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
	GameID          string
	OFEN            string
	Depth           int
	ResponseChannel chan *message.RoomMove
	// Done, if set, signals that the requesting room has been torn down so
	// the worker can drop its result instead of blocking forever on the
	// unbuffered ResponseChannel.
	Done <-chan struct{}
	Ctx  channel.SocketContext
}

// DeployRequest is a request for the engine to choose a bot's blind home-rank
// deployment. Like EngineRequest it is served off the room goroutine; the chosen
// board-order placement is returned on ResponseChannel, which the caller sizes
// (buffered) so this send never blocks even if the deploy phase has already
// ended (see room.handleDeploy).
type DeployRequest struct {
	Color           octad.Color
	ResponseChannel chan *message.RoomBotDeploy
}

// EngineDispatcher is a dispatcher for engine evaluation requests
type EngineDispatcher struct {
	Requests       chan EngineRequest
	DeployRequests chan DeployRequest
}

var instance EngineDispatcher

// UpEngine brings the engine dispatcher online
func UpEngine() {
	instance = EngineDispatcher{
		Requests:       make(chan EngineRequest),
		DeployRequests: make(chan DeployRequest),
	}
	go instance.run()
}

// SubmitEngine submits a request to the engine dispatcher
func SubmitEngine(request EngineRequest) {
	instance.Requests <- request
}

// SubmitDeploy submits a bot deploy-selection request to the engine dispatcher
func SubmitDeploy(request DeployRequest) {
	instance.DeployRequests <- request
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
		}
	}
}

// deployWorker chooses the bot's blind home-rank arrangement via the engine and
// returns it on the request's response channel.
func (d *EngineDispatcher) deployWorker(r DeployRequest) {
	util.DebugFlag("dispatch", str.CEng, "deploy request received for %s, selecting..", r.Color)

	placement := engine.SelectDeployment(r.Color)

	util.DebugFlag("dispatch", str.CEng, "deploy selected %s for %s", placement, r.Color)

	// the response channel is buffered by the caller, so this send never blocks
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

	move := engine.Search(r.OFEN, r.Depth, engine.MinimaxAB)

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
