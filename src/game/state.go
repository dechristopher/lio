package game

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
