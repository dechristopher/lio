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
	// IsSpectator marks a viewer with no seat in the room: the room page
	// renders watch-only (no game controls, White-oriented board) and the
	// client JS suppresses all move input.
	IsSpectator bool
	// WhiteIsBot / BlackIsBot report each seat's bot status by color — the
	// spectator view labels clocks by color rather than You/Opponent, so the
	// relative OpponentIsBot (meaningless for a non-player) doesn't apply.
	WhiteIsBot  bool
	BlackIsBot  bool
	VariantName string
	Variant     variant.Variant
	IsCreator   bool
	IsJoining   bool
	// Public reports whether the challenge is listed in the home-page Open
	// Challenges feed (vs a private, link-only challenge).
	Public      bool
	CancelToken string
	JoinToken   string
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

// RoomDrawEval carries the engine's verdict on a human's draw offer in a bot
// game: whether the bot accepts the draw. It is the draw-offer analogue of
// RoomMove — produced by the engine dispatcher and consumed by the room's
// game-ongoing handler — and is tagged with the game and position it was
// evaluated for so a verdict that arrives after the position changed (a move
// landed) is dropped instead of ending the wrong game.
type RoomDrawEval struct {
	GameID string
	OFEN   string
	Accept bool
}

type RoomControlType int

const (
	Rematch RoomControlType = iota
	Cancel
	Resign
	Draw
)
