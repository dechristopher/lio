package util

import (
	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lioctad/config"
	"github.com/dechristopher/lioctad/env"
)

// HandleTemplate will execute the http template engine
// with the given template, name, data, and status
func HandleTemplate(
	c *fiber.Ctx,
	template string,
	name string,
	data interface{},
	status int,
) error {
	return c.Status(status).Render(
		template,
		genPageModel(name, data),
		"layouts/main")
}

// PageModel contains runtime information that
// can be used during page template rendering
type pageModel struct {
	Env      env.Env
	Version  string
	CacheKey string
	PageName string
	Data     interface{}
}

// GenPageModel generates the global page model
func genPageModel(name string, data interface{}) pageModel {
	return pageModel{
		Env:      env.GetEnv(),
		Version:  config.Version,
		CacheKey: config.CacheKey,
		PageName: name,
		Data:     data,
	}
}
