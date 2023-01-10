package room

import (
	"github.com/dechristopher/lio/channel"
	wsv1 "github.com/dechristopher/lio/proto"
	"google.golang.org/protobuf/proto"
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// handleGameOver handles game finalization and rematch prompts
func (r *Instance) handleGameOver() {
	cleanupTimer := time.NewTimer(gameOverExpiryTime)
	defer cleanupTimer.Stop()

	meta := channel.SocketContext{
		Channel: r.ID,
		MT:      2,
	}

	roomSocketMap := channel.Map.GetSockMap(r.ID)
	connectionListener := roomSocketMap.Listen()
	defer roomSocketMap.UnListen(connectionListener)

	for {
		select {
		case <-cleanupTimer.C:
			// no rematch agreed to, clean up
			util.DebugFlag("room", str.CRoom, "[%s] no rematch, room over", r.ID)
			err := r.event(EventNoRematch)
			if err != nil {
				panic(err)
			}
			return
		case numConnections := <-connectionListener:
			util.DebugFlag("room", str.CRoom, "[%s] room player count changed: %d", r.ID, numConnections)
			websocketMessage := wsv1.WebsocketMessage{Data: &wsv1.WebsocketMessage_RematchPayload{RematchPayload: &wsv1.RematchPayload{
				BothPlayersPresent: r.BothPlayersConnected(),
				WhiteRequested:     r.rematch[octad.White],
				BlackRequested:     r.rematch[octad.Black],
			}}}

			payload, err := proto.Marshal(&websocketMessage)
			if err != nil {
				util.Error(str.CChan, "error encoding rematch message err=%s", err.Error())
			}

			channel.Broadcast(payload, meta)
		case control := <-r.controlChannel:
			if control.Type == message.Rematch {
				util.DebugFlag("room", str.CRoom, "[%s] processing rematch request", r.ID)
				blackPlayer, whitePlayer := r.players.GetPlayers()
				// track agreement for player looked up via context. auto-agree if either player is a bot
				if whitePlayer.ID == control.Ctx.UID || whitePlayer.IsBot {
					r.rematch.Agree(octad.White)
				}
				if blackPlayer.ID == control.Ctx.UID || blackPlayer.IsBot {
					r.rematch.Agree(octad.Black)
				}

				if r.rematch.Agreed() {
					util.DebugFlag("room", str.CRoom, "[%s] rematch agreed, restarting room", r.ID)
					err := r.event(EventRematchAgreed)
					if err != nil {
						panic(err)
					}

					// wait 1 second
					t := time.NewTimer(time.Second)
					<-t.C

					// flip board and reset game
					r.flipBoard()
					r.game, err = game.NewOctadGame(r.params.GameConfig)
					if err != nil {
						panic(err)
					}

					// reset rematch flags
					r.rematch = player.NewAgreement()

					websocketMessage := wsv1.WebsocketMessage{Data: &wsv1.WebsocketMessage_RematchPayload{RematchPayload: &wsv1.RematchPayload{
						RematchReady: true,
					}}}

					payload, err := proto.Marshal(&websocketMessage)
					if err != nil {
						util.Error(str.CChan, "error encoding rematch message err=%s", err.Error())
					}

					// broadcast rematch start
					channel.Broadcast(payload, meta)

					return
				} else {
					util.DebugFlag("room", str.CRoom, "[%s] rematch requested by player: %+v", r.ID, r.rematch)
					websocketMessage := wsv1.WebsocketMessage{Data: &wsv1.WebsocketMessage_RematchPayload{RematchPayload: &wsv1.RematchPayload{
						BothPlayersPresent: true, // is it fine to assume?
						BlackRequested:     r.rematch[octad.Black],
						WhiteRequested:     r.rematch[octad.White],
					}}}

					payload, err := proto.Marshal(&websocketMessage)
					if err != nil {
						util.Error(str.CChan, "error encoding rematch message err=%s", err.Error())
					}

					// broadcast rematch state change
					channel.Broadcast(payload, meta)
				}
			}
		}
	}
}
