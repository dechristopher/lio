package handlers

import (
	"github.com/dechristopher/octad"
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lioctad/game"
	"github.com/dechristopher/lioctad/player"
	"github.com/dechristopher/lioctad/room"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/variant"
)

// RoomHandler executes the room page template
func RoomHandler(c *fiber.Ctx) error {
	roomInstance, err := room.Get(c.Params("id"))
	if err != nil || roomInstance == nil {
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	bid := c.Cookies("bid")

	// signal to room that this player is joining
	isPlayer, isSpectator := roomInstance.Join(bid)

	if isPlayer {
		// get template payload for player
		payload := roomInstance.GenPlayerPayload(bid)
		// render template
		return util.HandleTemplate(c, "room",
			payload.VariantName, payload, 200)
	} else if isSpectator {
		// spectator
		// TODO signal to JS that this player is a spectator
		// only receive game updates
		// only able to draw on board and scroll moves

		// TODO spectator page template
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	} else {
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}
}

func NewRoomHumanHandler(c *fiber.Ctx) error {
	bid := c.Cookies("bid")

	params := room.Params{
		Players: make(player.Players),
		GameConfig: game.OctadGameConfig{
			Variant: variant.HalfOneBlitz,
		},
	}

	params.Players[octad.White] = &player.Player{
		ID: bid,
	}

	params.Players[octad.Black] = &player.Player{
		ID: "", // to be set by second joining player
	}

	instance, err := room.Create(params)

	if err != nil {
		util.Error(str.CRoom, "failed to create room via human handler: %s", err.Error())
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	util.Info(str.CRoom, "user %s created room %s vs human", bid, instance.ID)

	return c.Redirect("/" + instance.ID)
}

func NewRoomComputerHandler(c *fiber.Ctx) error {
	bid := c.Cookies("bid")

	params := room.Params{
		Players: make(player.Players),
		GameConfig: game.OctadGameConfig{
			Variant: variant.HalfOneBlitz,
		},
	}

	params.Players[octad.White] = &player.Player{
		ID: bid,
	}

	params.Players[octad.Black] = &player.Player{
		IsBot: true,
	}

	instance, err := room.Create(params)
	if err != nil {
		util.Error(str.CRoom, "failed to create room via computer handler: %s", err.Error())
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	util.Info(str.CRoom, "user %s created room %s vs computer", bid, instance.ID)

	return c.Redirect("/" + instance.ID)
}
