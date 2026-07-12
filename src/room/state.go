package room

import "github.com/looplab/fsm"

// State of a current OctadRoom
type State string

// Game states constants
const (
	StateInit              State = "init"
	StateWaitingForPlayers State = "waiting_for_players"
	StateGameReady         State = "game_ready"
	StateDeploy            State = "deploy"
	StateGameOngoing       State = "game_ongoing"
	StateGameOver          State = "game_over"
	StateRoomOver          State = "room_over"
)

var EventRoomInitialized = fsm.EventDesc{
	Name: "room_init",
	Src:  []string{string(StateInit)},
	Dst:  string(StateWaitingForPlayers),
}

var EventPlayerConnected = fsm.EventDesc{
	Name: "player_connected",
	Src:  []string{string(StateWaitingForPlayers)},
	Dst:  string(StateWaitingForPlayers),
}

var EventPlayersConnected = fsm.EventDesc{
	Name: "players_connected",
	Src:  []string{string(StateWaitingForPlayers)},
	Dst:  string(StateGameReady),
}

var EventStartGame = fsm.EventDesc{
	Name: "start_game",
	Src:  []string{string(StateGameReady)},
	Dst:  string(StateGameOngoing),
}

// EventStartDeploy begins the blind deploy phase. Used instead of EventStartGame
// when the room has the deploy pre-game enabled: players arrange their home rank
// before normal play begins.
var EventStartDeploy = fsm.EventDesc{
	Name: "start_deploy",
	Src:  []string{string(StateGameReady)},
	Dst:  string(StateDeploy),
}

// EventDeployComplete ends the deploy phase once both arrangements are in (or
// the deploy timer expires), starting the game from the assembled position.
var EventDeployComplete = fsm.EventDesc{
	Name: "deploy_complete",
	Src:  []string{string(StateDeploy)},
	Dst:  string(StateGameOngoing),
}

var EventWhiteWinsCheckmate = fsm.EventDesc{
	Name: "white_wins_checkmate",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventBlackWinsCheckmate = fsm.EventDesc{
	Name: "black_wins_checkmate",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventWhiteWinsTimeout = fsm.EventDesc{
	Name: "white_wins_timeout",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventBlackWinsTimeout = fsm.EventDesc{
	Name: "black_wins_timeout",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventWhiteWinsResignation = fsm.EventDesc{
	Name: "white_wins_resignation",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventBlackWinsResignation = fsm.EventDesc{
	Name: "black_wins_resignation",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventDrawInsufficient = fsm.EventDesc{
	Name: "draw_insufficient",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventDrawStalemate = fsm.EventDesc{
	Name: "draw_stalemate",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventDrawRepetition = fsm.EventDesc{
	Name: "draw_repetition",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventDraw25MoveRule = fsm.EventDesc{
	Name: "draw_25_move_rule",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventDrawAgreed = fsm.EventDesc{
	Name: "draw_agreed",
	Src:  []string{string(StateGameOngoing)},
	Dst:  string(StateGameOver),
}

var EventRematchAgreed = fsm.EventDesc{
	Name: "rematch_agreed",
	Src:  []string{string(StateGameOver)},
	Dst:  string(StateGameReady),
}

// EventNextGame advances an undecided race-to match to its next game: the
// auto-advance analogue of EventRematchAgreed, fired by the match interlude
// with no player agreement involved.
var EventNextGame = fsm.EventDesc{
	Name: "next_game",
	Src:  []string{string(StateGameOver)},
	Dst:  string(StateGameReady),
}

var EventNoRematch = fsm.EventDesc{
	Name: "no_rematch",
	Src:  []string{string(StateGameOver)},
	Dst:  string(StateRoomOver),
}

var EventPlayerAbandons = fsm.EventDesc{
	Name: "player_abandons",
	Src: []string{
		string(StateWaitingForPlayers),
		string(StateGameReady),
		string(StateDeploy),
		string(StateGameOngoing),
		string(StateGameOver),
	},
	Dst: string(StateRoomOver),
}

// newStateMachine returns a new finite state machine that helps
// to control the state flow of a game of octad on the site
func newStateMachine() *fsm.FSM {
	return newStateMachineAt(StateInit)
}

// newStateMachineAt returns the room state machine primed at the given state:
// the rehydration entry point, where the persisted state stands in for the
// transitions the original process already made. All other rooms start at
// StateInit via newStateMachine.
func newStateMachineAt(initial State) *fsm.FSM {
	return fsm.NewFSM(
		string(initial),
		fsm.Events{
			EventRoomInitialized,
			EventPlayerConnected,
			EventPlayersConnected,
			EventStartGame,
			EventStartDeploy,
			EventDeployComplete,
			EventWhiteWinsCheckmate,
			EventBlackWinsCheckmate,
			EventWhiteWinsTimeout,
			EventBlackWinsTimeout,
			EventWhiteWinsResignation,
			EventBlackWinsResignation,
			EventDrawInsufficient,
			EventDrawStalemate,
			EventDrawRepetition,
			EventDraw25MoveRule,
			EventDrawAgreed,
			EventRematchAgreed,
			EventNextGame,
			EventNoRematch,
			EventPlayerAbandons,
		},
		nil,
	)
}
