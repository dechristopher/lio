package message

import (
	"github.com/dechristopher/lioctad/channel"
	"github.com/dechristopher/lioctad/www/ws/proto"
)

type RoomTemplatePayload struct {
	PlayerColor   string
	OpponentColor string
	VariantName   string
}

type RoomMove struct {
	Player string
	GameID string // optional game identifier used for filtering out engine moves from previous games
	Move   proto.MovePayload
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
	Resign
	Draw
)
