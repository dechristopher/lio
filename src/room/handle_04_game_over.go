package room

import (
	"time"

	"github.com/dechristopher/octad"

	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// handleGameOver handles game finalization and rematch prompts
func (r *Instance) handleGameOver() {
	// 30 second timeout until rematch is unavailable
	rematchTimeout := time.NewTimer(30 * time.Second)
	defer rematchTimeout.Stop()

	for {
		select {
		case <-rematchTimeout.C:
			// no rematch agreed to, clean up
			util.DebugFlag("room", str.CRoom, "[%s] no rematch, room over", r.ID)

			//TODO, do we need to do anything here?
			// notify of match expiry / disable rematch buttons after timeout
			// signal to client to disconnect websockets

			err := r.event(EventNoRematch)
			if err != nil {
				panic(err)
			}
			return
		case control := <-r.controlChannel:
			if control.Type == message.Rematch {
				util.Debug(str.CRoom, "rematch %+v", control)
				// track agreement for player looked up via context
				util.DoBothColors(func(c octad.Color) {
					if r.players[c].ID == control.Player {
						util.Debug(str.CRoom, "rematch agree: %s", c.String())
						r.rematch.Agree(c)
					}
				})

				// auto-agree to rematch if either player is a bot
				util.DoBothColors(func(c octad.Color) {
					if r.players[c].IsBot {
						util.Debug(str.CRoom, "bot agree: %s", c.String())
						r.rematch.Agree(c)
					}
				})

				if r.rematch.Agreed() {
					util.Debug(str.CRoom, "rematch agreed!")
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
					r.rematch.Reset()

					return
				}
			}
		}
	}
}
