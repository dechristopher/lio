package game

import "github.com/looplab/fsm"

// State of a current OctadGame
type State int

// Game states constants
const (
	Active State = iota
	WhiteWinsCheckmate
	BlackWinsCheckmate
	DrawInsufficient
	DrawStalemate
	DrawAgreed
	DrawRepetition
	WhiteWinsResignation
	BlackWinsResignation
	WhiteWinsTimeout
	BlackWinsTimeout
)

func NewStateMachine() *fsm.FSM {
	return fsm.NewFSM(
		"",
		fsm.Events{
			{},
		},
		fsm.Callbacks{
			"": func(event *fsm.Event) {},
		},
	)
}
