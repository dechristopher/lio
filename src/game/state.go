package game

type State int

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
