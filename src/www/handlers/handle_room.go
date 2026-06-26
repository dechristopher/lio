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
	"github.com/dechristopher/lio/view"
)

type newRoomPayload struct {
	c             *fiber.Ctx
	variant       variant.Variant
	selectedColor octad.Color
	vsBot         bool
	public        bool
}

// redirect issues a client redirect that works for both normal and htmx
// requests. htmx form posts get an HX-Redirect header — a real browser
// navigation, so the destination (e.g. a room page) gets a true page load that
// boots the websocket and board — while normal requests fall back to a 302.
func redirect(c *fiber.Ctx, to string) error {
	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", to)
		return c.SendStatus(fiber.StatusOK)
	}
	return c.Redirect(to)
}

// getUserAndRoom returns the uid and room instance based on URL parameters
// and will redirect the user home or 404 if there is no UID or room
func getUserAndRoom(c *fiber.Ctx) (string, *room.Instance, error, bool) {
	uid := user.GetID(c)
	// turn away players/scripts/bots with no uid set
	if uid == "" {
		return "", nil, redirect(c, "/"), true
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
		return view.Render(c, 200, view.Room(view.RoomMeta(payload), payload))
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
		return redirect(c, "/#errJoin")
	}

	// attempt to join room
	if roomInstance.Join(uid, joinPayload.Token) {
		// broadcast message to waiting player(s)
		go roomInstance.NotifyWaiting()
		// redirect player to game room
		return redirect(c, fmt.Sprintf("/%s", roomInstance.ID))
	}

	// error out if join failed
	return redirect(c, "/#errJoinExpired")
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
		return redirect(c, "/#errCancel")
	}

	if !roomInstance.IsCreator(uid) || cancelPayload.Token != roomInstance.CancelToken() {
		return c.Redirect("/", fiber.StatusForbidden)
	}

	// cancel the game if we're allowed to
	if !roomInstance.Cancel() {
		return c.Redirect("/", fiber.StatusBadRequest)
	}

	// redirect home after room cancellation
	return redirect(c, "/")
}

// NewQuickRoomVsHuman creates a game against a human player with the default
// time control and randomized color. Quick games are a public seek by nature —
// there is no link to share with a specific opponent — so they are always listed.
func NewQuickRoomVsHuman(c *fiber.Ctx) error {
	return newRoom(newRoomPayload{
		c:             c,
		variant:       variant.HalfOneBlitz,
		selectedColor: util.RandomColor(),
		public:        true,
	})
}

// NewCustomRoomVsHuman creates a game against a human player with time control
// and color selected by the creator
func NewCustomRoomVsHuman(c *fiber.Ctx) error {

	selectedColor := octad.White

	payload := struct {
		TimeControl string `form:"time-control"`
		Color       string `form:"color"`
		// Public is the create-game "list publicly" toggle. The checkbox submits
		// "true" only when checked, so it defaults to false (private) when absent.
		Public bool `form:"public"`
	}{}

	if err := c.BodyParser(&payload); err != nil {
		util.Error(str.CRoom, "failed to create room via human handler: bad payload provided")
		return redirect(c, "/#error")
	}

	selectedVariant, ok := pools.Map[payload.TimeControl]
	if !ok {
		util.Error(str.CRoom, "failed to create room via human handler: invalid time control")
		return redirect(c, "/")
	}

	if payload.Color == "w" {
		selectedColor = octad.White
	} else if payload.Color == "b" {
		selectedColor = octad.Black
	} else if payload.Color == "r" {
		selectedColor = util.RandomColor()
	} else {
		util.Error(str.CRoom, "failed to create room via human handler: invalid color selected")
		return redirect(c, "/")
	}

	return newRoom(newRoomPayload{
		c:             c,
		variant:       selectedVariant,
		selectedColor: selectedColor,
		public:        payload.Public,
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
		return redirect(payload.c, "/")
	}

	// establish room parameters
	params := room.NewParams(uid, payload.variant)
	params.Public = payload.public

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
		return redirect(payload.c, "/")
	}

	util.Info(str.CRoom, "user %s created room %s, vsBot=%v", uid, instance.ID, payload.vsBot)

	// redirect to waiting room vs human, or game vs computer
	return redirect(payload.c, "/"+instance.ID)
}
