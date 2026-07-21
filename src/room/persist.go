package room

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/channel/handlers"
	"github.com/dechristopher/lio/clock"
	"github.com/dechristopher/lio/game"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/tv"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/variant"
)

// persistVersion is the room snapshot schema version. Bump it when the
// PersistedRoom shape changes incompatibly; Rehydrate rejects other versions
// (a stale snapshot from an old build is dropped, not misread).
//
// v2 (accounts, Phase 2): player.Snapshot gained the account fields
// (UserID/Username/RatingDisplay) and Params gained the creator account +
// Rated flag. In-flight v1 snapshots are dropped at the deploy that ships v2
// — a one-time loss of live rooms, so deploy in a quiet window.
const persistVersion = 2

// PersistedRoom is the serializable snapshot of a room for restart
// persistence: everything needed to rebuild the room actor after a process
// restart, per arch/STATE_PERSISTENCE_SCALING.md. Only rooms with both seats
// committed are persisted (see Persist), and per-game state is captured only
// past the first move (state game_ongoing/game_over); a room persisted at
// game_ready restores with a fresh game, re-running any deploy phase.
type PersistedRoom struct {
	V       int       `json:"v"`
	SavedAt time.Time `json:"at"`

	RoomID        string `json:"id"`
	Creator       string `json:"creator"`
	CreatorUserID *int64 `json:"creatorUid,omitempty"`
	CreatorName   string `json:"creatorName,omitempty"`
	State         State  `json:"state"`
	Public        bool   `json:"public,omitempty"`
	Rated         bool   `json:"rated,omitempty"`
	JoinToken     string `json:"jt,omitempty"`
	CancelToken   string `json:"ct,omitempty"`

	// params: the full variant definition is embedded (rather than a registry
	// key) so a snapshot never dangles on a renamed variant; clock.CTime
	// round-trips via its Marshal/UnmarshalJSON pair
	Variant    variant.Variant `json:"variant"`
	ParamsOFEN string          `json:"pofen,omitempty"`
	RaceTo     int             `json:"raceTo,omitempty"`
	Deploy     bool            `json:"deploy,omitempty"`
	Casual     bool            `json:"casual,omitempty"`
	BotPersona string          `json:"botPersona,omitempty"`

	White player.Snapshot `json:"white"`
	Black player.Snapshot `json:"black"`

	// per-game state; empty for a game_ready snapshot
	GameID    string          `json:"gameId,omitempty"`
	GameStart time.Time       `json:"gameStart,omitempty"`
	StartOFEN string          `json:"startOfen,omitempty"`
	Moves     []string        `json:"moves,omitempty"`
	MoveTimes []game.MoveTime `json:"moveTimes,omitempty"`
	Outcome   string          `json:"outcome,omitempty"`
	Method    uint8           `json:"method,omitempty"`
	Clock     clock.Snapshot  `json:"clock,omitempty"`

	HumanMoved   bool        `json:"humanMoved,omitempty"`
	DrawOffer    octad.Color `json:"drawOffer,omitempty"`
	DrawWhite    bool        `json:"drawW,omitempty"`
	DrawBlack    bool        `json:"drawB,omitempty"`
	RematchWhite bool        `json:"rematchW,omitempty"`
	RematchBlack bool        `json:"rematchB,omitempty"`

	// absolute deadlines; consumed relative on restore (a lapsed window is
	// floored/refreshed rather than instantly expiring the room)
	RematchDeadline  time.Time `json:"rematchDeadline,omitempty"`
	NextGameDeadline time.Time `json:"nextGameDeadline,omitempty"`
}

// Persist captures the room as a versioned JSON snapshot, or returns false for
// rooms that are not persistable: open challenges (waiting_for_players — a
// reconnecting client is redirected home instead) and rooms already tearing
// down. Safe to call from any goroutine (captures under stateMu); the clock
// snapshot of a running clock reads as-of-last-flip, which is exactly the
// restore policy (players are never charged for process downtime).
func (r *Instance) Persist() ([]byte, bool) {
	// normalize the live FSM state to a restorable one. Mid-deploy partial
	// arrangements are deliberately not persisted: a room captured in the
	// deploy phase restores at game_ready and re-runs the deploy from the top.
	var state State
	switch r.State() {
	case StateGameReady, StateDeploy:
		state = StateGameReady
	case StateGameOngoing, StateGameOver:
		state = StateGameOngoing // refined to game_over below iff decided
	default:
		return nil, false
	}

	r.stateMu.Lock()

	wp, bp := r.players[octad.White], r.players[octad.Black]
	if wp == nil || bp == nil {
		// no committed opponent yet; nothing worth restoring
		r.stateMu.Unlock()
		return nil, false
	}

	p := PersistedRoom{
		V:       persistVersion,
		SavedAt: time.Now(),

		RoomID:        r.ID,
		Creator:       r.creator,
		CreatorUserID: r.params.CreatorUserID,
		CreatorName:   r.params.CreatorName,
		Public:        r.public,
		Rated:         r.params.Rated,
		JoinToken:     r.joinToken,
		CancelToken:   r.cancelToken,

		Variant:    r.params.GameConfig.Variant,
		ParamsOFEN: r.params.GameConfig.OFEN,
		RaceTo:     r.params.RaceTo,
		Deploy:     r.params.Deploy,
		Casual:     r.params.Casual,
		BotPersona: r.params.BotPersona,

		White: wp.Snapshot(),
		Black: bp.Snapshot(),

		HumanMoved:   r.humanMoved,
		DrawOffer:    r.drawOffer,
		DrawWhite:    r.draw.AgreedBy(octad.White),
		DrawBlack:    r.draw.AgreedBy(octad.Black),
		RematchWhite: r.rematch.AgreedBy(octad.White),
		RematchBlack: r.rematch.AgreedBy(octad.Black),

		RematchDeadline:  r.rematchDeadline,
		NextGameDeadline: r.nextGameDeadline,
	}

	if state != StateGameReady {
		// the game's terminal outcome, not the FSM, decides ongoing vs over: a
		// snapshot taken in the decided-but-not-yet-transitioned sliver restores
		// into the game-over window rather than a dead "ongoing" game
		if r.game.Outcome() != octad.NoOutcome {
			state = StateGameOver
		}

		p.GameID = r.game.ID
		p.GameStart = r.game.Start
		p.StartOFEN = r.game.Positions()[0].String()
		p.Outcome = string(r.game.Outcome())
		p.Method = uint8(r.game.Method())
		p.Clock = r.game.Clock.Snapshot()
		for _, mov := range r.game.Moves() {
			p.Moves = append(p.Moves, mov.String())
		}
		p.MoveTimes = append(p.MoveTimes, r.game.MoveTimes...)
	}
	p.State = state

	r.stateMu.Unlock()

	data, err := json.Marshal(p)
	if err != nil {
		util.Error(str.CRoom, "[%s] snapshot marshal failed: %v", r.ID, err)
		return nil, false
	}
	return data, true
}

// Rehydrate rebuilds a room Instance from a Persist snapshot: game replayed
// from its starting OFEN, clock restored paused, players reseated with scores
// and match history, FSM primed at the persisted state. The returned room is
// inert — StartRehydrated registers it and starts its routine — so the boot
// path can rebuild every room before any of them begins running.
func Rehydrate(data []byte) (*Instance, error) {
	var p PersistedRoom
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("rehydrate: %w", err)
	}
	if p.V != persistVersion {
		return nil, fmt.Errorf("rehydrate %s: snapshot version %d, want %d",
			p.RoomID, p.V, persistVersion)
	}

	switch p.State {
	case StateGameReady, StateGameOngoing, StateGameOver:
	default:
		return nil, fmt.Errorf("rehydrate %s: state %q is not restorable",
			p.RoomID, p.State)
	}

	players := player.Players{
		octad.White: player.RestorePlayer(p.White),
		octad.Black: player.RestorePlayer(p.Black),
	}

	params := Params{
		Creator:       p.Creator,
		CreatorUserID: p.CreatorUserID,
		CreatorName:   p.CreatorName,
		Players:       players,
		GameConfig: game.OctadGameConfig{
			White:   p.White.ID,
			Black:   p.Black.ID,
			Variant: p.Variant,
			OFEN:    p.ParamsOFEN,
		},
		Public:     p.Public,
		Rated:      p.Rated,
		Deploy:     p.Deploy,
		RaceTo:     p.RaceTo,
		Casual:     p.Casual,
		BotPersona: p.BotPersona,
	}

	var g *game.OctadGame
	var err error
	if p.State == StateGameReady {
		// nothing per-game survives at game_ready: a fresh game (and clock),
		// exactly as if the room had just reached readiness; any deploy phase
		// re-runs from the top
		g, err = game.NewOctadGame(params.GameConfig)
	} else {
		g, err = game.RestoreOctadGame(params.GameConfig, p.GameID, p.GameStart,
			p.StartOFEN, p.Moves, p.MoveTimes,
			clock.Restore(p.Variant.Control, p.Clock))
		if err == nil {
			err = reapplyOutcome(g, octad.Outcome(p.Outcome), octad.Method(p.Method))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("rehydrate %s: %w", p.RoomID, err)
	}

	r := &Instance{
		ID:           p.RoomID,
		creator:      p.Creator,
		stateMachine: newStateMachineAt(p.State),
		params:       params,
		game:         g,

		stateChannel:    make(chan State, 1),
		moveChannel:     make(chan *message.RoomMove),
		controlChannel:  make(chan message.RoomControl, 2),
		deployChannel:   make(chan *message.RoomDeploy, deployChannelBuffer),
		drawEvalChannel: make(chan *message.RoomDrawEval, 1),
		done:            make(chan struct{}),

		players:   players,
		rematch:   player.NewAgreement(),
		draw:      player.NewAgreement(),
		drawOffer: p.DrawOffer,

		humanMoved: p.HumanMoved,
		public:     p.Public,

		joinToken:   p.JoinToken,
		cancelToken: p.CancelToken,

		rematchDeadline:  p.RematchDeadline,
		nextGameDeadline: p.NextGameDeadline,
	}

	if p.DrawWhite {
		r.draw.Agree(octad.White)
	}
	if p.DrawBlack {
		r.draw.Agree(octad.Black)
	}
	if p.RematchWhite {
		r.rematch.Agree(octad.White)
	}
	if p.RematchBlack {
		r.rematch.Agree(octad.Black)
	}

	switch p.State {
	case StateGameOngoing:
		// the restored clock is paused; handleGameOngoing resumes it once both
		// seats reconnect (or on the first move), never charging the downtime
		r.resumeClockPending = true

	case StateGameOver:
		if decided, _ := r.MatchDecided(); p.RaceTo > 0 && !decided {
			// undecided race-to interlude: a deadline that lapsed during the
			// restart is refreshed to a full interlude so returning players are
			// not instantly held to the missing-player forfeit grace
			if r.nextGameDeadline.Before(time.Now()) {
				r.nextGameDeadline = time.Now().Add(matchInterludeWindow)
			}
		} else if !players.HasBot() && !p.RematchDeadline.IsZero() {
			// human rematch window: re-enter with the remaining window, floored
			// at the disconnect grace and capped at the full window
			w := time.Until(p.RematchDeadline)
			if w < rematchDisconnectGrace {
				w = rematchDisconnectGrace
			}
			if w > rematchWindow {
				w = rematchWindow
			}
			r.restoredWindow = w
		}
		// bot games take a fresh analysis window (rematchDeadline is never set
		// for them), which is already handleGameOver's default
	}

	return r, nil
}

// StartRehydrated registers a rehydrated room and starts its routine: the
// restore-path analogue of Create's tail + init, minus the FSM init event
// (the state machine was primed at the persisted state). Split from Rehydrate
// so the boot path can rebuild every room before any routine runs, and so
// tests can inspect a rehydrated Instance without starting goroutines.
func (r *Instance) StartRehydrated() error {
	if _, exists := rooms.Load(r.ID); exists {
		return fmt.Errorf("rehydrate %s: room already exists", r.ID)
	}

	// track sockmaps for each room channel type, as Create does
	for _, channelType := range roomChannelTypes {
		channel.Map.GetSockMap(fmt.Sprintf("%s%s", channelType, r.ID))
	}

	// crowd presence handling, as Create does
	go handlers.HandleCrowd(r.ID, r.PlayerIDs, func(spec int) {
		tv.Publish(tv.Event{Kind: tv.Crowd, RoomID: r.ID, Watchers: spec})
	})

	rooms.Store(r.ID, r)

	// re-announce a live game to the home-page TV grid: the tv.Start publish in
	// handleGameReady already ran in the previous process, and a restored
	// ongoing room re-enters the FSM past it
	if r.State() == StateGameOngoing {
		tv.Publish(r.tvEvent(tv.Start))
	}

	go r.routine()

	util.Info(str.CRoom, "[%s] room rehydrated (state %s)", r.ID, r.State())
	return nil
}

// reapplyOutcome re-applies a declared (non-board-derived) result to a
// replayed game. Board-derived outcomes — checkmate, stalemate, insufficient
// material, repetition, the 25-move rule — re-arise from the replay itself;
// resignations (including clock flags, which octad records as resignations)
// and agreed draws exist only as declarations and must be replayed onto the
// rebuilt game. Any disagreement between the replayed and persisted outcome
// means a corrupt snapshot and fails the rehydration.
func reapplyOutcome(g *game.OctadGame, outcome octad.Outcome, method octad.Method) error {
	if outcome == octad.NoOutcome || g.Outcome() == outcome {
		return nil
	}
	if g.Outcome() != octad.NoOutcome {
		return fmt.Errorf("replay produced %s, snapshot says %s", g.Outcome(), outcome)
	}

	switch {
	case outcome == octad.Draw && method == octad.DrawOffer:
		if err := g.Draw(octad.DrawOffer); err != nil {
			return fmt.Errorf("re-applying agreed draw: %w", err)
		}
	case outcome == octad.WhiteWon:
		g.Resign(octad.Black)
	case outcome == octad.BlackWon:
		g.Resign(octad.White)
	default:
		return fmt.Errorf("cannot re-apply outcome %s (method %d)", outcome, method)
	}

	if g.Outcome() != outcome {
		return fmt.Errorf("outcome re-apply produced %s, want %s", g.Outcome(), outcome)
	}
	return nil
}
