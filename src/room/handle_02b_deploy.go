package room

import (
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/tv"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// handleDeploy runs the blind deploy phase: both players privately arrange their
// home rank, then the game begins from the assembled position. A bot deploys
// immediately with a random legal arrangement (until the engine chooses one in a
// later phase). When both arrangements are in — or the deploy timer expires, in
// which case any missing arrangement is auto-filled with the standard ordering —
// the game is rebuilt from the deployed OFEN and normal play begins.
func (r *Instance) handleDeploy() {
	util.DebugFlag("room", str.CRoom, "[%s] deploy phase started", r.ID)

	// record the deploy deadline so a (re)connecting client can be told the
	// correct remaining time via DeployStateMessage, and reset the committed
	// arrangements from any prior game so reconnect state starts clean
	r.stateMu.Lock()
	r.deployDeadline = time.Now().Add(deployTimeout)
	r.deployed = make(map[octad.Color]Deployment, 2)
	r.stateMu.Unlock()

	// announce the deploy phase to everyone; clients enter deploy mode
	channel.Broadcast(r.deployMessage(int(deployTimeout/time.Second)), channel.SocketContext{Channel: r.ID, MT: 1})

	deployTimer := time.NewTimer(deployTimeout)
	defer deployTimer.Stop()

	// periodically re-announce the live phase so a client that missed the
	// single phase-start broadcast is pulled in within one interval, and so
	// in-phase clients can reconcile lock state (and a lost confirm — the
	// announce carries both sides' locked flags, so a client whose own color
	// isn't locked despite having confirmed knows to resend)
	announce := time.NewTicker(deployAnnounceInterval)
	defer announce.Stop()

	// collected arrangements by color
	got := make(map[octad.Color]Deployment, 2)

	r.stateMu.Lock()
	botColor := r.players.GetBotColor()
	r.stateMu.Unlock()

	// a bot's arrangement is chosen by the engine off this goroutine (144 shallow
	// searches) so the deploy timer and cancellation stay responsive; the result
	// arrives on botDeployCh. The channel is buffered per call so a late result
	// from a previous deploy phase can never leak into this one, and so the
	// dispatcher's send never blocks even if this phase ends before it returns.
	var botDeployCh chan *message.RoomBotDeploy
	if botColor != octad.NoColor {
		botDeployCh = make(chan *message.RoomBotDeploy, 1)
		r.requestEngineDeploy(botColor, botDeployCh)
	}

	// collect submissions until both are in or the timer expires. A nil
	// botDeployCh (no bot) simply never selects its case.
	for len(got) < 2 {
		select {
		case sub := <-r.deployChannel:
			r.stateMu.Lock()
			_, color := r.players.Lookup(sub.Player)
			r.stateMu.Unlock()
			if color == octad.NoColor {
				continue // not a seated player
			}
			d, err := parseDeployment(sub.Order)
			if err != nil {
				util.DebugFlag("room", str.CRoom, "[%s] rejected deploy from %s: %v", r.ID, sub.Player, err)
				// resync the player to the deploy state so they can retry
				channel.Unicast(r.DeployStateMessage(sub.Player), sub.Ctx)
				continue
			}
			got[color] = d
			r.recordAndLock(color, d)
			util.DebugFlag("room", str.CRoom, "[%s] %s deployed", r.ID, color)
		case bot := <-botDeployCh:
			d := deploymentFromPlacement(bot.Color, bot.Placement)
			got[bot.Color] = d
			r.recordAndLock(bot.Color, d)
			util.DebugFlag("room", str.CRoom, "[%s] bot (%s) deployed %s", r.ID, bot.Color, d.order())
		case <-announce.C:
			channel.Broadcast(r.deployAnnounceMessage(), channel.SocketContext{Channel: r.ID, MT: 1})
		case <-deployTimer.C:
			util.DebugFlag("room", str.CRoom, "[%s] deploy timer expired, auto-filling", r.ID)
			r.deployAndStart(got, botColor)
			return
		case control := <-r.controlChannel:
			// a cancel during deploy tears the room down like any other phase
			if control.Type == message.Cancel {
				r.cancelled = true
				return
			}
		}
	}

	r.deployAndStart(got, botColor)
}

// deployAndStart auto-fills any missing arrangement, assembles the deployed
// OFEN, swaps in a fresh game starting from it, starts the clock, reveals the
// position, and transitions to the live game. If the bot is to move first, its
// opening move is requested after the transition.
func (r *Instance) deployAndStart(got map[octad.Color]Deployment, botColor octad.Color) {
	// auto-fill any side that never submitted with the standard ordering
	util.DoBothColors(func(c octad.Color) {
		if _, ok := got[c]; !ok {
			got[c] = standardDeployment
		}
	})

	ofen, err := assembleDeployedOFEN(got[octad.White], got[octad.Black])
	if err != nil {
		// deployments are validated on submit and the auto-fill is always legal,
		// so this is effectively unreachable; fall back to the standard start
		util.Error(str.CRoom, "[%s] deploy assembly failed: %v", r.ID, err)
		ofen, _ = assembleDeployedOFEN(standardDeployment, standardDeployment)
	}

	util.DebugFlag("room", str.CRoom, "[%s] deploy complete, starting from %s", r.ID, ofen)

	// rebuild the game from the deployed position, preserving variant and players
	r.stateMu.Lock()
	cfg := r.params.GameConfig
	cfg.OFEN = ofen
	ng, gerr := game.NewOctadGame(cfg)
	if gerr != nil {
		r.stateMu.Unlock()
		util.Error(str.CRoom, "[%s] failed to build deployed game: %v", r.ID, gerr)
		r.abandoned = true
		if eErr := r.event(EventPlayerAbandons); eErr != nil {
			panic(eErr)
		}
		return
	}
	r.game = ng
	r.game.ToMove = ng.Position().Turn()
	// a freshly deployed game has had no human move yet
	r.humanMoved = false
	// the deploy phase is over; clear the deadline and committed arrangements
	r.deployDeadline = time.Time{}
	r.deployed = nil
	clk := r.game.Clock
	r.stateMu.Unlock()

	// the game proper begins; start white's clock
	clk.Start()

	// reveal the assembled position to everyone and announce the game on TV
	channel.Broadcast(r.CurrentGameStateMessage(false, true), channel.SocketContext{Channel: r.ID, MT: 1})
	tv.Publish(r.tvEvent(tv.Start))

	// transition to the live game before kicking the bot's opening move so the
	// game-ongoing handler is the one listening on moveChannel
	if err := r.event(EventDeployComplete); err != nil {
		panic(err)
	}

	// when the bot plays White it owns the opening move
	if botColor == octad.White {
		util.DebugFlag("room", str.CRoom, "[%s] bot to move, requesting opening move", r.ID)
		r.requestEngineMove()
	}
}

// recordAndLock stores a side's committed arrangement so a (re)connecting client
// can be told its own confirmed order (and both sides' locked status), and tells
// everyone that this color has locked in — driving the opponent's and spectators'
// "locked in" indicator. The broadcast happens outside the lock (it is network
// I/O) and is safe from the deploy handler goroutine.
func (r *Instance) recordAndLock(color octad.Color, d Deployment) {
	r.stateMu.Lock()
	if r.deployed == nil {
		r.deployed = make(map[octad.Color]Deployment, 2)
	}
	r.deployed[color] = d
	r.stateMu.Unlock()

	channel.Broadcast(r.lockMessage(color), channel.SocketContext{Channel: r.ID, MT: 1})
}

// playerIDsLocked returns the current white/black player ids. The caller must
// hold stateMu.
func (r *Instance) playerIDsLocked() (white, black string) {
	if p := r.players[octad.White]; p != nil {
		white = p.ID
	}
	if p := r.players[octad.Black]; p != nil {
		black = p.ID
	}
	return white, black
}

// colorName renders an octad color as the lowercase name the client protocol
// uses ("white"/"black").
func colorName(c octad.Color) string {
	if c == octad.Black {
		return "black"
	}
	return "white"
}

// deployMessage builds the wire message that announces the blind deploy phase
// to clients, carrying the given number of seconds remaining plus the current
// white/black player ids so a client can determine its own side (important after
// a rematch swaps colors — the board orientation class in the DOM is stale).
// Like every outbound deploy message it names the pre-deploy game (GameID) so
// the client can anchor reveal recognition and reject stale phase messages.
func (r *Instance) deployMessage(seconds int) []byte {
	r.stateMu.Lock()
	white, black := r.playerIDsLocked()
	gid := r.game.ID
	r.stateMu.Unlock()

	msg := proto.Message{
		Tag: string(proto.DeployTag),
		Data: proto.DeployPayload{
			Active:  true,
			Seconds: seconds,
			White:   white,
			Black:   black,
			GameID:  gid,
		},
		ProtoVersion: proto.DeployPayloadVersion,
	}
	return msg.Please()
}

// deployAnnounceMessage builds the periodic re-broadcast of the live deploy
// phase: remaining seconds, player ids, both sides' locked status, and the
// pre-deploy game id. It carries no per-recipient fields (Order/Confirmed are
// unicast-only, from DeployStateMessage), so a confirmed client reconciles a
// lost submission off its own color's locked flag instead.
func (r *Instance) deployAnnounceMessage() []byte {
	r.stateMu.Lock()
	deadline := r.deployDeadline
	white, black := r.playerIDsLocked()
	_, lockedWhite := r.deployed[octad.White]
	_, lockedBlack := r.deployed[octad.Black]
	gid := r.game.ID
	r.stateMu.Unlock()

	remaining := 0
	if d := time.Until(deadline); d > 0 {
		remaining = int(d.Seconds())
	}

	msg := proto.Message{
		Tag: string(proto.DeployTag),
		Data: proto.DeployPayload{
			Active:      true,
			Seconds:     remaining,
			White:       white,
			Black:       black,
			LockedWhite: lockedWhite,
			LockedBlack: lockedBlack,
			GameID:      gid,
		},
		ProtoVersion: proto.DeployPayloadVersion,
	}
	return msg.Please()
}

// lockMessage builds the per-submission broadcast telling clients that the given
// color has committed its arrangement, so the opponent and spectators can show a
// "locked in" indicator without re-entering deploy mode.
func (r *Instance) lockMessage(color octad.Color) []byte {
	r.stateMu.Lock()
	white, black := r.playerIDsLocked()
	gid := r.game.ID
	r.stateMu.Unlock()

	msg := proto.Message{
		Tag: string(proto.DeployTag),
		Data: proto.DeployPayload{
			Active: true,
			Locked: colorName(color),
			White:  white,
			Black:  black,
			GameID: gid,
		},
		ProtoVersion: proto.DeployPayloadVersion,
	}
	return msg.Please()
}

// DeployStateMessage returns the deploy-phase wire message for a (re)connecting
// client, carrying the remaining deploy seconds plus — for a seated player who
// already committed — their own confirmed order and locked state, and both
// sides' locked status. It is safe to call from WS handler goroutines and is how
// a refreshed/reconnected client re-enters deploy mode (restoring its prior
// arrangement) instead of seeing a stale board.
//
// Returns nil once the phase is no longer live: the caller's State()==StateDeploy
// check races deployAndStart (the room stays in StateDeploy until
// EventDeployComplete fires near its end), and a deploy-state message built from
// the already-cleared fields — zero seconds, no locks — arriving after the
// reveal would push a client back into deploy mode over a live game. deployAndStart
// clears deployDeadline under stateMu before revealing, so a nil here tells the
// caller to fall through to the current board state instead.
func (r *Instance) DeployStateMessage(uid string) []byte {
	r.stateMu.Lock()
	deadline := r.deployDeadline
	white, black := r.playerIDsLocked()
	_, color := r.players.Lookup(uid)
	var order string
	var confirmed bool
	if color != octad.NoColor {
		if d, ok := r.deployed[color]; ok {
			order = d.order()
			confirmed = true
		}
	}
	_, lockedWhite := r.deployed[octad.White]
	_, lockedBlack := r.deployed[octad.Black]
	gid := r.game.ID
	r.stateMu.Unlock()

	if deadline.IsZero() {
		return nil
	}

	remaining := 0
	if d := time.Until(deadline); d > 0 {
		remaining = int(d.Seconds())
	}

	msg := proto.Message{
		Tag: string(proto.DeployTag),
		Data: proto.DeployPayload{
			Active:      true,
			Seconds:     remaining,
			White:       white,
			Black:       black,
			Order:       order,
			Confirmed:   confirmed,
			LockedWhite: lockedWhite,
			LockedBlack: lockedBlack,
			GameID:      gid,
		},
		ProtoVersion: proto.DeployPayloadVersion,
	}
	return msg.Please()
}
