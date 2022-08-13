package handlers

import (
	"github.com/dechristopher/lioctad/game"
	"github.com/dechristopher/lioctad/room"
	"github.com/dechristopher/lioctad/str"
	"github.com/dechristopher/lioctad/util"
	"github.com/dechristopher/lioctad/variant"
	"github.com/dechristopher/octad"
	"github.com/gofiber/fiber/v2"
)

type RoomTemplatePayload struct {
	PlayerColor   string
	OpponentColor string
	VariantName   string
}

// RoomHandler executes the room page template
func RoomHandler(c *fiber.Ctx) error {
	roomInstance, err := room.Get(c.Params("id"))
	if err != nil || roomInstance == nil {
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	browserId := c.Cookies("bid")

	// signal to room that this player is joining as P2
	joined, isSpectator := roomInstance.Join(browserId)

	if isSpectator {
		// spectator
		// TODO signal to JS that this player is a spectator
		// only receive game updates
		// only able to draw on board and scroll moves

		// TODO spectator page template
	}

	if joined {
		var playerColor octad.Color
		if roomInstance.Player1 == browserId {
			playerColor = roomInstance.P1Color
		} else {
			playerColor = roomInstance.P1Color.Other()
		}

		payload := RoomTemplatePayload{
			PlayerColor:   playerColor.String(),
			OpponentColor: playerColor.Other().String(),
			VariantName:   roomInstance.Game().Variant.Name + " " + string(roomInstance.Game().Variant.Group),
		}

		return util.HandleTemplate(c, "room",
			payload.VariantName, payload, 200)
	} else {
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}
}

func NewRoomHumanHandler(c *fiber.Ctx) error {
	bid := c.Cookies("bid")

	instance, err := room.Create(room.Params{
		Player1: bid,
		P1Color: octad.White,
		GameConfig: game.OctadGameConfig{
			Variant: variant.OneTwoRapid,
		},
	})
	if err != nil {
		util.Error(str.CRoom, "failed to create room via human handler: %s", err.Error())
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	util.Info(str.CRoom, "user %s created room %s vs human", bid, instance.ID)

	return c.Redirect("/" + instance.ID)
}

func NewRoomRobotHandler(c *fiber.Ctx) error {
	bid := c.Cookies("bid")

	instance, err := room.Create(room.Params{
		Player1: bid,
		P1Color: octad.White,
		P2Bot:   true,
		GameConfig: game.OctadGameConfig{
			Variant: variant.OneTwoRapid,
		},
	})
	if err != nil {
		util.Error(str.CRoom, "failed to create room via human handler: %s", err.Error())
		return c.Redirect("/", fiber.StatusTemporaryRedirect)
	}

	util.Info(str.CRoom, "user %s created room %s vs human", bid, instance.ID)

	return c.Redirect("/" + instance.ID)
}
