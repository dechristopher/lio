package room

import (
	"time"

	"github.com/dechristopher/lioctad/channel"
	"github.com/dechristopher/lioctad/game"
	"github.com/dechristopher/lioctad/message"
)

// handleGameOver handles game finalization and rematch prompts
func (r *Instance) handleGameOver() {
	// auto-rematch
	go func() {
		t := time.NewTimer(time.Second * 2)
		<-t.C

		// manually set rematch true
		r.P1Rematch = true
		r.P2Rematch = true

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
				if r.Player1 == control.Ctx.BID {
					r.P1Rematch = true
				}

				if r.Player2 == control.Ctx.BID {
					r.P2Rematch = true
				}

				// automatically rematch if P2 is a bot
				if r.P1Rematch && r.P2Bot {
					r.P2Rematch = true
				}

				if r.P1Rematch && r.P2Rematch {
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
					r.P1Rematch = false
					r.P2Rematch = false

					return
				}
			}
		}
	}
}
