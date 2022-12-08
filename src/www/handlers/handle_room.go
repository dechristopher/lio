package handlers

import (
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/pools"
	wsv1 "github.com/dechristopher/lio/proto"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/variant"
	"github.com/dechristopher/octad"
	"github.com/gofiber/fiber/v2"
	"google.golang.org/protobuf/proto"
)

type newRoomPayload struct {
	c             *fiber.Ctx
	variant       *wsv1.Variant
	selectedColor octad.Color
	vsBot         bool
}

// getUserAndRoom returns the uid and room instance based on URL parameters
// and will redirect the user home or 404 if there is no UID or room
func getUserAndRoom(c *fiber.Ctx) (string, *room.Instance, error, bool) {
	uid := user.GetID(c)
	// turn away players/scripts/bots with no uid set
	if uid == "" {
		return "", nil, c.Redirect("/"), true
	}

	// grab room instance
	roomInstance, err := room.Get(c.Params("id"))
	if err != nil || roomInstance == nil {
		// continue to 404 page if room not found
		return "", nil, c.Status(fiber.StatusNotFound).Next(), true
	}

	return uid, roomInstance, nil, false
}

// RoomStatusesHandler returns important room information
func RoomStatusesHandler(c *fiber.Ctx) error {
	rooms := room.GetAll()
	var roomStatuses []message.RoomStatusPayload
	for _, currRoom := range rooms {
		roomStatuses = append(roomStatuses, currRoom.GenStatusPayload())
	}

	return c.Status(200).JSON(roomStatuses)
}

// RoomHandler returns important room information
func RoomHandler(c *fiber.Ctx) error {
	uid, roomInstance, err, redirected := getUserAndRoom(c)
	if err != nil || redirected {
		return err
	}

	return c.Status(200).JSON(roomInstance.GenLobbyPayload(uid))
}

// RoomJoinHandler joins the player to the room
func RoomJoinHandler(c *fiber.Ctx) error {
	uid, roomInstance, err, redirected := getUserAndRoom(c)
	if err != nil || redirected {
		return err
	}

	// attempt to join room
	if roomInstance.Join(uid) {
		// broadcast message to waiting player(s)
		go roomInstance.NotifyWaiting()
	}

	// TODO should we error out if join failed? for successes is there something else that makes more sense than a redirect?
	return c.Redirect("/" + roomInstance.ID)
}

// RoomCancelHandler cancels the room immediately
func RoomCancelHandler(c *fiber.Ctx) error {
	uid, roomInstance, err, redirected := getUserAndRoom(c)
	if err != nil || redirected {
		return err
	}

	if !roomInstance.IsCreator(uid) {
		return c.Redirect("/", fiber.StatusForbidden)
	}

	// cancel the game if we're allowed to
	if !roomInstance.Cancel() {
		return c.Redirect("/", fiber.StatusBadRequest)
	}

	// redirect home after room cancellation
	return c.Redirect("/")
}

// NewQuickRoomVsHuman creates a game against a human player with the default
// time control and randomized color
func NewQuickRoomVsHuman(c *fiber.Ctx) error {
	return newRoom(newRoomPayload{
		c:             c,
		variant:       variant.HalfTwoRapid,
		selectedColor: util.RandomColor(),
	})
}

// NewCustomRoomVsHuman creates a game against a human player with time control
// and color selected by the creator
func NewCustomRoomVsHuman(c *fiber.Ctx) error {
	selectedColor := octad.NoColor

	payload := &wsv1.NewCustomRoomPayload{}
	if err := proto.Unmarshal(c.Body(), payload); err != nil {
		util.Error(str.CRoom, "failed to create room via human handler: bad payload provided")
		return c.Redirect("/#error")
	}

	selectedVariant, ok := pools.Map[payload.VariantHtmlName]
	if !ok {
		util.Error(str.CRoom, "failed to create room via human handler: invalid time control")
		return c.Redirect("/")
	}

	if payload.PlayerColor == wsv1.PlayerColor_PLAYER_COLOR_WHITE {
		selectedColor = octad.White
	} else if payload.PlayerColor == wsv1.PlayerColor_PLAYER_COLOR_BLACK {
		selectedColor = octad.Black
	} else if payload.PlayerColor == wsv1.PlayerColor_PLAYER_COLOR_UNSPECIFIED {
		selectedColor = util.RandomColor()
	} else {
		util.Error(str.CRoom, "failed to create room via human handler: invalid color selected")
		// TODO a better way to handle errors?
		//fiber.NewError(fiber.StatusBadRequest, "Test")
		return c.Redirect("/")
	}

	return newRoom(newRoomPayload{
		c:             c,
		variant:       selectedVariant,
		selectedColor: selectedColor,
	})
}

// NewRoomVsComputer creates a new game against a computer opponent with the
// default time control and randomized color
func NewRoomVsComputer(c *fiber.Ctx) error {
	return newRoom(newRoomPayload{
		c:             c,
		variant:       variant.HalfTwoRapid,
		selectedColor: util.RandomColor(),
		vsBot:         true,
	})
}

// newRoom handles room creation and the validation of room payload parameters
func newRoom(payload newRoomPayload) error {
	uid := user.GetID(payload.c)

	if uid == "" {
		// TODO prevent anonymous users from creating games when we have accounts
		return payload.c.Redirect("/")
	}

	// establish room parameters
	params := room.NewParams(uid, payload.variant)

	// set creating player ID in players map
	params.Players[payload.selectedColor] = &player.Player{
		ID: uid,
	}

	// configure room with player to join via URL
	toJoin := player.ToJoin
	params.Players[payload.selectedColor.Other()] = &toJoin

	// set bot=true if game is configured with computer opponent
	if payload.vsBot {
		params.Players[payload.selectedColor.Other()].IsBot = true
	}

	// create room and handle resultant errors
	instance, err := room.Create(params)
	if err != nil {
		util.Error(str.CRoom, "failed to create room: %s", err.Error())
		return payload.c.Redirect("/")
	}

	util.Info(str.CRoom, "user %s created room %s, vsBot=%v", uid, instance.ID, payload.vsBot)

	// redirect to waiting room vs human, or game vs computer
	return payload.c.Redirect("/" + instance.ID)
}
