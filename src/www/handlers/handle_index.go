package handlers

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/pools"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/variant"
)

var cachedIndex []byte

type indexData struct {
	Pools map[variant.Group][]variant.Variant
}

// IndexHandler executes the home page template
func IndexHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "index",
		"Free Online Octad", indexData{
			Pools: pools.RatingPools,
		}, 200)
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
