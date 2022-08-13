package handlers

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lioctad/config"
	"github.com/dechristopher/lioctad/util"
)

type status struct {
	Version  string  `json:"v"`      // current lio version
	Uptime   float64 `json:"uptime"` // uptime in seconds
	BootTime int64   `json:"boot"`   // time started, unix timestamp
}

// StatusHandler returns a JSON object with status info
func StatusHandler(c *fiber.Ctx) error {
	return c.JSON(status{
		Version:  config.Version,
		Uptime:   util.TimeSinceBoot().Seconds(),
		BootTime: config.BootTime.UnixNano(),
	})
}
