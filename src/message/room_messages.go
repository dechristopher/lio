package message

import (
	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/variant"
	"github.com/dechristopher/lio/www/ws/proto"
)

type RoomStatusPayload struct {
	RoomID    string
	RoomState proto.RoomState
	Variant   variant.Variant
	Players   player.Players
}

type RoomLobbyPayload struct {
	RoomID      string
	RoomState   proto.RoomState
	PlayerColor string
	Variant     variant.Variant
	IsCreator   bool
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
