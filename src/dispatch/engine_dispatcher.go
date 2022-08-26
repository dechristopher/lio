package dispatch

import (
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
	Ctx             channel.SocketContext
}

// EngineDispatcher is a dispatcher for engine evaluation requests
type EngineDispatcher struct {
	Requests chan EngineRequest
}

var instance EngineDispatcher

// UpEngine brings the engine dispatcher online
func UpEngine() {
	instance = EngineDispatcher{Requests: make(chan EngineRequest)}
	go instance.run()
}

// SubmitEngine submits a request to the engine dispatcher
func SubmitEngine(request EngineRequest) {
	instance.Requests <- request
}

// dispatcher for engine evaluation requests from games against the engine
func (d *EngineDispatcher) run() {
	util.Debug(str.CEng, str.DEngOk)
	for {
		select {
		case request := <-d.Requests:
			go d.worker(request)
		}
	}
}

// worker to actually crunch, find, and return the engine move
func (d *EngineDispatcher) worker(r EngineRequest) {
	// ensure upstream handlers know this move is from a bot
	r.Ctx.IsBot = true
	r.Ctx.BID = ""

	util.DebugFlag("dispatch", str.CEng, "[%s] request received, searching(%d)..", r.OFEN, r.Depth)

	move := engine.Search(r.OFEN, r.Depth, engine.MinimaxAB)

	util.DebugFlag("dispatch", str.CEng, "[%s] found move %s", r.OFEN, move.Move.String())

	// write response move to given channel
	r.ResponseChannel <- &message.RoomMove{
		Player: "engine",
		GameID: r.GameID,
		Move: proto.MovePayload{
			Clock: proto.ClockPayload{},
			UOI:   move.Move.String(),
		},
		Ctx: r.Ctx,
	}
}
