package handlers

import (
	"fmt"

	"github.com/dechristopher/octad/v2"
	"github.com/gofiber/fiber/v3"

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
	c             fiber.Ctx
	variant       variant.Variant
	selectedColor octad.Color
	vsBot         bool
	public        bool
	// raceTo makes the room a race-to match (see room.Params.RaceTo); zero is
	// the classic single-game-with-rematches experience.
	raceTo int
}

// raceToChoices is the allowlist of race-to targets offered by the create-game
// modal; anything else in the form is rejected as a tampered payload.
var raceToChoices = map[int]bool{0: true, 3: true, 5: true, 7: true}

// redirect issues a client redirect that works for both normal and htmx
// requests. htmx form posts get an HX-Redirect header — a real browser
// navigation, so the destination (e.g. a room page) gets a true page load that
// boots the websocket and board — while normal requests fall back to a 302.
func redirect(c fiber.Ctx, to string) error {
	if c.Get("HX-Request") == "true" {
		c.Set("HX-Redirect", to)
		return c.SendStatus(fiber.StatusOK)
	}
	return c.Redirect().To(to)
}

// getUserAndRoom returns the uid and room instance based on URL parameters
// and will redirect the user home or 404 if there is no UID or room
func getUserAndRoom(c fiber.Ctx) (string, *room.Instance, error, bool) {
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
func RoomHandler(c fiber.Ctx) error {
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
		// watch-only room page: the flag suppresses the game controls in the
		// view and all move input in the client JS; the socket layer tags the
		// connection independently (ws.connHandler) so game-affecting frames
		// are dropped server-side no matter what the page claims
		payload.IsSpectator = true
		return view.Render(c, 200, view.Room(view.RoomMeta(payload), payload))
	} else {
		// no spectators allowed
		return c.Redirect().To("/#noSpec")
	}
}

// RoomJoinHandler joins the player to the room
func RoomJoinHandler(c fiber.Ctx) error {
	uid, roomInstance, err, redirected := getUserAndRoom(c)
	if err != nil || redirected {
		return err
	}

	joinPayload := struct {
		Token string `form:"join_token"`
	}{}

	err = c.Bind().Body(&joinPayload)
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
func RoomCancelHandler(c fiber.Ctx) error {
	uid, roomInstance, err, redirected := getUserAndRoom(c)
	if err != nil || redirected {
		return err
	}

	cancelPayload := struct {
		Token string `form:"cancel_token"`
	}{}

	err = c.Bind().Body(&cancelPayload)
	if err != nil {
		return redirect(c, "/#errCancel")
	}

	if !roomInstance.IsCreator(uid) || cancelPayload.Token != roomInstance.CancelToken() {
		return c.Redirect().Status(fiber.StatusForbidden).To("/")
	}

	// cancel the game if we're allowed to
	if !roomInstance.Cancel() {
		return c.Redirect().Status(fiber.StatusBadRequest).To("/")
	}

	// redirect home after room cancellation
	return redirect(c, "/")
}

// NewQuickRoomVsHuman creates a game against a human player with the default
// time control and randomized color. Quick games are a public seek by nature —
// there is no link to share with a specific opponent — so they are always listed.
func NewQuickRoomVsHuman(c fiber.Ctx) error {
	return newRoom(newRoomPayload{
		c:             c,
		variant:       variant.HalfOneBlitz,
		selectedColor: util.RandomColor(),
		public:        true,
	})
}

// NewCustomRoom creates a custom game from the create-game modal: a time control
// and color chosen by the creator, against either a human or the computer. The
// resolved variant (classic or its blind-deploy twin) arrives in the
// time-control field; the modal's Classic/Deploy toggle picks which. A human
// game may be listed as a public open challenge; a bot game never is.
func NewCustomRoom(c fiber.Ctx) error {
	payload := struct {
		TimeControl string `form:"time-control"`
		Color       string `form:"color"`
		// Opponent selects the opponent kind: "computer" for a bot game, anything
		// else (default "human") for a human game.
		Opponent string `form:"opponent"`
		// Public is the create-game "list publicly" toggle. The checkbox submits
		// "true" only when checked, so it defaults to false (private) when absent.
		Public bool `form:"public"`
		// RaceTo is the match-length choice: 0 for a single game (the default),
		// or a race to N points. Validated against raceToChoices below.
		RaceTo int `form:"race-to"`
	}{}

	if err := c.Bind().Body(&payload); err != nil {
		util.Error(str.CRoom, "failed to create custom room: bad payload provided")
		return redirect(c, "/#error")
	}

	selectedVariant, ok := pools.Map[payload.TimeControl]
	if !ok {
		util.Error(str.CRoom, "failed to create custom room: invalid time control %q", payload.TimeControl)
		return redirect(c, "/")
	}

	var selectedColor octad.Color
	switch payload.Color {
	case "w":
		selectedColor = octad.White
	case "b":
		selectedColor = octad.Black
	case "r":
		selectedColor = util.RandomColor()
	default:
		util.Error(str.CRoom, "failed to create custom room: invalid color selected")
		return redirect(c, "/")
	}

	vsBot := payload.Opponent == "computer"

	if !raceToChoices[payload.RaceTo] {
		util.Error(str.CRoom, "failed to create custom room: invalid race-to %d", payload.RaceTo)
		return redirect(c, "/")
	}
	raceTo := payload.RaceTo
	if vsBot {
		// matches are human-vs-human only for now; force bot games single-game
		raceTo = 0
	}

	return newRoom(newRoomPayload{
		c:             c,
		variant:       selectedVariant,
		selectedColor: selectedColor,
		vsBot:         vsBot,
		// bot games are never public open challenges
		public: payload.Public && !vsBot,
		raceTo: raceTo,
	})
}

// NewRoomVsComputer creates a new game against a computer opponent. With no
// query parameters it uses the default time control and a randomized color (the
// home-page quick game). The optional tc (time-control HTMLName) and color
// (w/b/r) query params let a finished game's client spin up a "same settings"
// rematch into a fresh room — a bot game's rematch does not reuse its (possibly
// already torn-down) room, so it navigates here instead.
func NewRoomVsComputer(c fiber.Ctx) error {
	selectedVariant := variant.HalfOneBlitz
	if tc := c.Query("tc"); tc != "" {
		if v, ok := pools.Map[tc]; ok {
			selectedVariant = v
		}
	}

	selectedColor := util.RandomColor()
	switch c.Query("color") {
	case "w":
		selectedColor = octad.White
	case "b":
		selectedColor = octad.Black
	}

	return newRoom(newRoomPayload{
		c:             c,
		variant:       selectedVariant,
		selectedColor: selectedColor,
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
	params.RaceTo = payload.raceTo

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
