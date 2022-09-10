package handlers

import (
	"fmt"

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

type newRoomPayload struct {
	c             *fiber.Ctx
	variant       variant.Variant
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

// RoomHandler executes the room page template
func RoomHandler(c *fiber.Ctx) error {
	uid, roomInstance, err, redirected := getUserAndRoom(c)
	if err != nil || redirected {
		return err
	}

	// figure out how user is allowed to join this room
	asPlayer, asSpectator := roomInstance.CanJoin(uid)

	// get template payload for user
	payload := roomInstance.GenTemplatePayload(uid)

	if asPlayer { // user is player
		// if game waiting state, enable waiting room / join room templates
		// but only if both players are humans
		roomInstance.HandlePreGame(uid, &payload)

		// render template
		return util.HandleTemplate(c, roomTemplate,
			payload.VariantName, payload, 200)
	} else if asSpectator { // user is spectator
		// TODO signal to JS that this player is a spectator
		// by excluding some player-specific scripts
		// --->
		// only receive game updates
		// only able to draw on board and scroll moves

		// payload.IsSpectator = true?

		// TODO spectator page template
		return c.Redirect("/#TODO")
	} else {
		// no spectators allowed
		return c.Redirect("/#noSpec")
	}
}

// RoomJoinHandler joins the player to the room
func RoomJoinHandler(c *fiber.Ctx) error {
	uid, roomInstance, err, redirected := getUserAndRoom(c)
	if err != nil || redirected {
		return err
	}

	joinPayload := struct {
		Token string `form:"join_token"`
	}{}

	err = c.BodyParser(&joinPayload)
	if err != nil {
		return c.Redirect("/#errJoin")
	}

	// attempt to join room
	if roomInstance.Join(uid, joinPayload.Token) {
		// broadcast message to waiting player(s)
		go roomInstance.NotifyWaiting()
		// redirect player to game room
		return c.Redirect(fmt.Sprintf("/%s", roomInstance.ID))
	}

	// error out if join failed
	return c.Redirect("/#errJoinExpired")
}

// RoomCancelHandler cancels the room immediately
func RoomCancelHandler(c *fiber.Ctx) error {
	uid, roomInstance, err, redirected := getUserAndRoom(c)
	if err != nil || redirected {
		return err
	}

	cancelPayload := struct {
		Token string `form:"cancel_token"`
	}{}

	err = c.BodyParser(&cancelPayload)
	if err != nil {
		return c.Redirect("/#errCancel")
	}

	if !roomInstance.IsCreator(uid) || cancelPayload.Token != roomInstance.CancelToken() {
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
		return c.Redirect("/#error")
	}

	selectedVariant, ok := pools.Map[payload.TimeControl]
	if !ok {
		util.Error(str.CRoom, "failed to create room via human handler: invalid time control")
		return c.Redirect("/")
	}

	if payload.Color == "w" {
		selectedColor = octad.White
	} else if payload.Color == "b" {
		selectedColor = octad.Black
	} else if payload.Color == "r" {
		selectedColor = util.RandomColor()
	} else {
		util.Error(str.CRoom, "failed to create room via human handler: invalid color selected")
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
		variant:       variant.HalfOneBlitz,
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
