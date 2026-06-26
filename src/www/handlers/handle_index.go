package handlers

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/message"
	"github.com/dechristopher/lio/pools"
	"github.com/dechristopher/lio/presence"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/view"
)

var cachedIndex []byte

// IndexHandler renders the home page
func IndexHandler(c *fiber.Ctx) error {
	presence.Touch(user.GetID(c))
	challenges, stats := homeActivity()
	return view.Render(c, 200, view.Index(
		view.PageMeta("Free Online Octad"), pools.RatingPools, challenges, stats))
}

// HomeActivityHandler renders the live home-activity fragment (site stats, open
// challenges) polled by htmx from the home page. The live-games grid is no
// longer part of this fragment — it streams over /socket/tv (see tvWidget).
func HomeActivityHandler(c *fiber.Ctx) error {
	presence.Touch(user.GetID(c))
	challenges, stats := homeActivity()
	return view.Render(c, 200, view.HomeActivity(challenges, stats))
}

// homeActivity gathers the shared home-page activity data and resolves the
// site-wide "online now" count by unioning the in-room humans with the recent
// home-page viewers (the calling handler having just Touch'd itself into the
// latter). The live-games slice from HomeListing is unused here now (the TV
// widget streams that), but stats.LiveGames still reflects the live count.
func homeActivity() ([]message.OpenChallenge, message.SiteStats) {
	_, challenges, stats, present := room.HomeListing()
	stats.Playing = presence.Online(present)
	return challenges, stats
}

// SPAHandlerInit creates the SPA handler to serve index.html for all
// requests that don't hit WS, API or static assets directly
func SPAHandlerInit(staticFs http.FileSystem) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		if len(cachedIndex) == 0 || !env.IsProd() {
			file, err := staticFs.Open("index.html")
			if err != nil {
				return err
			}

			i, err := file.Stat()
			if err != nil {
				return err
			}

			b := make([]byte, i.Size())
			_, err = file.Read(b)

			if err != nil {
				return err
			}

			if len(b) == 0 {
				return nil
			}

			cachedIndex = b
		} else {

		}

		// return index.html cached or not
		return c.Type("html", "utf-8").
			Status(200).Send(cachedIndex)
	}
}
