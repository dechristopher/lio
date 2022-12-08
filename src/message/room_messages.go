package message

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/player"
	wsv1 "github.com/dechristopher/lio/proto"
)

type RoomStatusPayload struct {
	RoomID    string
	RoomState wsv1.RoomState
	Variant   *wsv1.Variant
	Players   player.Players
}

type RoomMove struct {
	Player string
	GameID string // optional game identifier used for filtering out engine moves from previous games
	Move   *wsv1.MovePayload
	Ctx    channel.SocketContext
}

type RoomControl struct {
	Player string
	Type   RoomControlType
	Ctx    channel.SocketContext
}

type RoomControlType int

const (
	Rematch RoomControlType = iota
	Cancel
	Join
	Resign
	Draw
)
