package room

import (
	"github.com/dechristopher/lio/www/ws/proto"
	"github.com/looplab/fsm"
)

// Game states constants
const (
	StateInit              proto.RoomState = "init"
	StateWaitingForPlayers proto.RoomState = "waiting_for_players"
	StateGameReady         proto.RoomState = "game_ready"
	StateGameOngoing       proto.RoomState = "game_ongoing"
	StateGameOver          proto.RoomState = "game_over"
	StateRoomOver          proto.RoomState = "room_over"
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
		string(StateGameOngoing),
		string(StateGameOver),
	},
	Dst: string(StateRoomOver),
}

// newStateMachine returns a new finite state machine that helps
// to control the state flow of a game of octad on the site
func newStateMachine() *fsm.FSM {
	return fsm.NewFSM(
		string(StateInit),
		fsm.Events{
			EventRoomInitialized,
			EventPlayerConnected,
			EventPlayersConnected,
			EventStartGame,
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
			EventNoRematch,
			EventPlayerAbandons,
		},
		nil,
	)
}
