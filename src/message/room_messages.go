package message

import (
	"github.com/dechristopher/octad/v2"

	"github.com/dechristopher/lio/channel"
	"github.com/dechristopher/lio/variant"
	"github.com/dechristopher/lio/www/ws/proto"
)

type RoomTemplatePayload struct {
	RoomID        string
	PlayerColor   string
	OpponentColor string
	OpponentIsBot bool
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

// RoomDeploy carries a player's blind deploy-phase submission: a four-character
// home-rank ordering (k/n/p letters) from that player's own perspective.
type RoomDeploy struct {
	Player string
	Order  string
	Ctx    channel.SocketContext
}

// RoomBotDeploy carries the engine's chosen blind deploy arrangement for a bot
// player, in board order (index i = file a+i on the bot's home rank). It is the
// deploy-phase analogue of RoomMove: produced by the engine dispatcher and
// consumed by the room's deploy handler, which maps it to the bot's own
// perspective before committing it.
type RoomBotDeploy struct {
	Color     octad.Color
	Placement [4]octad.PieceType
}

type RoomControlType int

const (
	Rematch RoomControlType = iota
	Cancel
	Resign
	Draw
)
