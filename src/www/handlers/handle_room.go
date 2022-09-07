package handlers

import (
	"github.com/dechristopher/octad"
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lio/player"
	"github.com/dechristopher/lio/pools"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/variant"
)

const roomTemplate = "room"

type NewRoomPayload struct {
	c             *fiber.Ctx
	variant       variant.Variant
	selectedColor octad.Color
	vsBot         bool
}

// RoomHandler executes the room page template
func RoomHandler(c *fiber.Ctx) error {
	roomInstance, err := room.Get(c.Params("id"))
	if err != nil || roomInstance == nil {
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	uid := user.GetID(c)

	// turn away players with no uid set
	if uid == "" {
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	// signal to room that this player is joining
	isPlayer, isSpectator := roomInstance.Join(uid)

	// get template payload for user
	payload := roomInstance.GenTemplatePayload(uid)

	if isPlayer {
		// user is player

		// if game waiting state, enable waiting room / join room templates
		if roomInstance.State() == room.StateWaitingForPlayers {
			payload.IsCreator = roomInstance.IsCreator(uid)
			payload.IsJoining = !payload.IsCreator
		}

		// render template
		return util.HandleTemplate(c, roomTemplate,
			payload.VariantName, payload, 200)
	} else if isSpectator {
		// user is spectator
		// TODO signal to JS that this player is a spectator
		// by excluding some player-specific scripts
		// --->
		// only receive game updates
		// only able to draw on board and scroll moves

		// payload.IsSpectator = true

		// TODO spectator page template
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	} else {
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}
}

// NewQuickRoomVsHuman creates a game against a human player with the default
// time control and randomized color
func NewQuickRoomVsHuman(c *fiber.Ctx) error {
	return NewRoom(NewRoomPayload{
		c:             c,
		variant:       variant.HalfOneBlitz,
		selectedColor: util.RandomColor(),
	})
}

// NewCustomRoomVsHuman creates a game against a human player with time control
// and color selected by the creator
func NewCustomRoomVsHuman(c *fiber.Ctx) error {

	selectedColor := octad.White

	payload := struct {
		TimeControl string `form:"time-control"`
		Color       string `form:"color"`
	}{}

	if err := c.BodyParser(&payload); err != nil {
		util.Error(str.CRoom, "failed to create room via human handler: bad payload provided")
		return c.Redirect("/#error", fiber.StatusTemporaryRedirect)
	}

	selectedVariant, ok := pools.Map[payload.TimeControl]
	if !ok {
		util.Error(str.CRoom, "failed to create room via human handler: invalid time control")
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	if payload.Color == "w" {
		selectedColor = octad.White
	} else if payload.Color == "b" {
		selectedColor = octad.Black
	} else if payload.Color == "r" {
		selectedColor = util.RandomColor()
	} else {
		util.Error(str.CRoom, "failed to create room via human handler: invalid color selected")
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	return NewRoom(NewRoomPayload{
		c:             c,
		variant:       selectedVariant,
		selectedColor: selectedColor,
	})
}

// NewRoom handles room creation and the validation of room payload parameters
func NewRoom(payload NewRoomPayload) error {
	uid := user.GetID(payload.c)

	if uid == "" {
		// TODO prevent anonymous users from creating games when we have accounts
		return payload.c.Redirect("/", fiber.StatusTemporaryRedirect)
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
		return payload.c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	util.Info(str.CRoom, "user %s created room %s, vsBot=%v", uid, instance.ID, payload.vsBot)

	// redirect to waiting room vs human, or game vs computer
	return payload.c.Redirect("/" + instance.ID)
}

// NewRoomVsComputer creates a new game against a computer opponent with the
// default time control and randomized color
func NewRoomVsComputer(c *fiber.Ctx) error {
	return NewRoom(NewRoomPayload{
		c:             c,
		variant:       variant.HalfOneBlitz,
		selectedColor: util.RandomColor(),
		vsBot:         true,
	})
}
