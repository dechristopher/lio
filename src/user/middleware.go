package user

import (
	"encoding/json"

	"github.com/gofiber/fiber/v2"

	"github.com/dechristopher/lio/crypt"
	"github.com/dechristopher/lio/env"
)

const (
	contextCookieName = "lio"
	uidCookieName     = "uid"
)

// ContextMiddleware evaluates and/or sets the user
// context for incoming requests
func ContextMiddleware(c *fiber.Ctx) error {
	var userContext = new(Context)
	var err error

	// set cookies for future use
	if c.Cookies(contextCookieName) == "" {
		// generate new anonymous context
		userContext = Anonymous()
		// marshal anonymous context to encrypted JSON
		contextCookie, err := userContext.MarshalJSON()
		if err != nil {
			return c.Next()
		}

		// set secure enclave cookie
		c.Cookie(&fiber.Cookie{
			Name:     contextCookieName,
			Value:    string(contextCookie),
			Path:     "/",
			MaxAge:   0,
			Secure:   !env.IsLocal(),
			HTTPOnly: true,
			SameSite: "Strict",
		})

		// set uid cookie so JS-land knows what's going on
		c.Cookie(&fiber.Cookie{
			Name:     uidCookieName,
			Value:    userContext.ID,
			Path:     "/",
			MaxAge:   0,
			Secure:   !env.IsLocal(),
			HTTPOnly: false,
			SameSite: "Strict",
		})
	} else {
		// decrypt the encrypted enclave context
		decryptedJson, errDecrypt := crypt.Decrypt([]byte(c.Cookies(contextCookieName)))
		if errDecrypt != nil {
			return wipeContext(c)
		}
		// decrypt and unmarshal the enclave context cookie
		err = json.Unmarshal(decryptedJson, userContext)
		if err != nil || userContext.ID != c.Cookies(uidCookieName) {
			return wipeContext(c)
		}
	}

	// set user context for later handlers
	c.SetUserContext(userContext)

	return c.Next()
}

// wipeContext clears the context cookies and redirects the user home
func wipeContext(c *fiber.Ctx) error {
	c.ClearCookie(contextCookieName)
	c.ClearCookie(uidCookieName)
	return c.Redirect("/", fiber.StatusTemporaryRedirect)
}
