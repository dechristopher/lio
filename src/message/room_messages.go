package message

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/variant"
	"github.com/dechristopher/lio/www/ws/proto"
)

type RoomTemplatePayload struct {
	RoomID        string
	PlayerColor   string
	OpponentColor string
	VariantName   string
	Variant       variant.Variant
	IsCreator     bool
	IsJoining     bool
	CancelToken   string
	JoinToken     string
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
	Cancel
	Resign
	Draw
)
