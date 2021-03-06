package handlers

import (
	"net/http"

	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lioctad/env"
	"github.com/dechristopher/lioctad/util"
)

var cachedIndex []byte

// IndexHandler executes the home page template
func IndexHandler(c *fiber.Ctx) error {
	return util.HandleTemplate(c, "index",
		"Coming Soon", nil, 200)
}

// SPAHandlerInit creates the SPA handler to serve index.html for all
// requests that don't hit WS, API or static assets directly
func SPAHandlerInit(staticFs http.FileSystem) func(c *fiber.Ctx) error {
	return func(c *fiber.Ctx) error {
		if len(cachedIndex) == 0 || env.IsDev() {
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
