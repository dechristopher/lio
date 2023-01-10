package room

import (
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/looplab/fsm"
)

var EventRoomInitialized = fsm.EventDesc{
	Name: "room_init",
	Src:  []string{wsv1.RoomState_ROOM_STATE_INIT.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_WAITING_FOR_PLAYERS.String(),
}

var EventPlayersConnected = fsm.EventDesc{
	Name: "players_connected",
	Src:  []string{wsv1.RoomState_ROOM_STATE_WAITING_FOR_PLAYERS.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_READY.String(),
}

var EventStartGame = fsm.EventDesc{
	Name: "start_game",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_READY.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String(),
}

var EventWhiteWinsCheckmate = fsm.EventDesc{
	Name: "white_wins_checkmate",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventBlackWinsCheckmate = fsm.EventDesc{
	Name: "black_wins_checkmate",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventWhiteWinsTimeout = fsm.EventDesc{
	Name: "white_wins_timeout",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventBlackWinsTimeout = fsm.EventDesc{
	Name: "black_wins_timeout",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventWhiteWinsResignation = fsm.EventDesc{
	Name: "white_wins_resignation",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventBlackWinsResignation = fsm.EventDesc{
	Name: "black_wins_resignation",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventDrawInsufficient = fsm.EventDesc{
	Name: "draw_insufficient",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventDrawStalemate = fsm.EventDesc{
	Name: "draw_stalemate",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventDrawRepetition = fsm.EventDesc{
	Name: "draw_repetition",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventDraw25MoveRule = fsm.EventDesc{
	Name: "draw_25_move_rule",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventDrawAgreed = fsm.EventDesc{
	Name: "draw_agreed",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
}

var EventRematchAgreed = fsm.EventDesc{
	Name: "rematch_agreed",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_OVER.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_GAME_READY.String(),
}

var EventNoRematch = fsm.EventDesc{
	Name: "no_rematch",
	Src:  []string{wsv1.RoomState_ROOM_STATE_GAME_OVER.String()},
	Dst:  wsv1.RoomState_ROOM_STATE_ROOM_OVER.String(),
}

var EventPlayerAbandons = fsm.EventDesc{
	Name: "player_abandons",
	Src: []string{
		wsv1.RoomState_ROOM_STATE_WAITING_FOR_PLAYERS.String(),
		wsv1.RoomState_ROOM_STATE_GAME_READY.String(),
		wsv1.RoomState_ROOM_STATE_GAME_ONGOING.String(),
		wsv1.RoomState_ROOM_STATE_GAME_OVER.String(),
	},
	Dst: wsv1.RoomState_ROOM_STATE_ROOM_OVER.String(),
}

// newStateMachine returns a new finite state machine that helps
// to control the state flow of a game of octad on the site
func newStateMachine() *fsm.FSM {
	return fsm.NewFSM(
		wsv1.RoomState_ROOM_STATE_INIT.String(),
		fsm.Events{
			EventRoomInitialized,
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
