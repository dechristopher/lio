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
		case _ = <-connectionListener:
			blackPlayer := r.players[octad.Black]
			whitePlayer := r.players[octad.White]
			blackPlayerPresent := blackPlayer.IsBot || roomSocketMap.Has(blackPlayer.ID)
			whitePlayerPresent := whitePlayer.IsBot || roomSocketMap.Has(whitePlayer.ID)
			bothPlayersPresent := blackPlayerPresent && whitePlayerPresent

			websocketMessage := wsv1.WebsocketMessage{Data: &wsv1.WebsocketMessage_RematchPayload{RematchPayload: &wsv1.RematchPayload{
				BothPlayersPresent: bothPlayersPresent,
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
				// track agreement for player looked up via context. auto-agree if either player is a bot
				util.DoBothColors(func(color octad.Color) {
					if r.players[color].ID == control.Ctx.UID || r.players[color].IsBot {
						r.rematch.Agree(color)
					}
				})

				if r.rematch.Agreed() {
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
