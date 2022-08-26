package room

import (
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/util"
)

// handleGameOver handles game finalization and rematch prompts
func (r *Instance) handleGameOver() {
	// auto-rematch
	go func() {
		t := time.NewTimer(time.Second * 2)
		<-t.C

		// manually set rematch true
		util.DoBothColors(func(color octad.Color) {
			r.rematch.Agree(color)
		})

		// trigger routine
		r.controlChannel <- message.RoomControl{
			Type: message.Rematch,
			Ctx: channel.SocketContext{
				Channel: r.ID,
				MT:      1,
			},
		}
	}()

	for {
		select {
		case control := <-r.controlChannel:
			if control.Type == message.Rematch {
				// track agreement for player looked up via context
				util.DoBothColors(func(c octad.Color) {
					if r.players[c].ID == control.Ctx.BID {
						r.rematch.Agree(c)
					}
				})

				// auto-agree to rematch if either player is a bot
				util.DoBothColors(func(c octad.Color) {
					if r.players[c].IsBot {
						r.rematch.Agree(c)
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

					return
				}
			}
		}
	}
}
