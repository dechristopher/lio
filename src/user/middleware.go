package user

import (
	"encoding/json"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/crypt"
	"github.com/dechristopher/lio/env"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

const (
	contextCookieName = "lio"
	uidCookieName     = "uid"
)

// ContextMiddleware evaluates and/or sets the user
// context for incoming requests
func ContextMiddleware(c fiber.Ctx) error {
	// WebSocket upgrades authenticate with the cookies they present, or not at
	// all: never mint a fresh identity for a socket and never wipe cookies +
	// redirect. iOS Safari intermittently omits cookies from WS upgrade
	// requests (see ios-deploy-confirm-bug / webkit.org #255524), so a minted
	// identity here would seat the connection as a spectator whose game frames
	// are silently dropped, and wipeContext's 302 would break the handshake and
	// trap the client in a reconnect loop. On any failure we simply pass the
	// request through with no context; ws.connHandler rejects the empty-uid
	// upgrade with a dedicated close code the client knows how to recover from.
	if strings.HasPrefix(c.Path(), "/socket") {
		if enclave := c.Cookies(contextCookieName); enclave != "" {
			if decryptedJson, errDecrypt := crypt.Decrypt([]byte(enclave)); errDecrypt == nil {
				userContext := new(Context)
				if json.Unmarshal(decryptedJson, userContext) == nil &&
					userContext.ID == c.Cookies(uidCookieName) {
					c.SetContext(userContext)
				}
			}
		}
		return c.Next()
	}

	var userContext = new(Context)
	var err error

	// set cookies for future use
	if c.Cookies(contextCookieName) == "" {
		// generate new anonymous context
		userContext = Anonymous()
		// marshal anonymous context to encrypted JSON
		contextCookie, err := userContext.MarshalJSON()
		if err != nil {
			// an unresolvable crypto key lands here: every visitor stays
			// cookie-less and every WS upgrade is identity-less (a 4001
			// close loop), so scream instead of failing silently
			util.Error(str.CUser, str.EIdentityMint, err.Error())
			return c.Next()
		}

		// set secure enclave cookie
		//
		// SameSite is Lax, not Strict: Strict cookies are unreliably attached to
		// WebSocket upgrade requests by WebKit (all iOS browsers), which is how
		// game sockets ended up seated as spectators. Lax still withholds the
		// cookies from cross-site subresource requests and POSTs; these cookies
		// only identify anonymous sessions, so Lax's top-level-GET exposure is
		// acceptable.
		c.Cookie(&fiber.Cookie{
			Name:     contextCookieName,
			Value:    string(contextCookie),
			Path:     "/",
			MaxAge:   0,
			Secure:   !env.IsLocal(),
			HTTPOnly: true,
			SameSite: "Lax",
		})

		// set uid cookie so JS-land knows what's going on
		c.Cookie(&fiber.Cookie{
			Name:     uidCookieName,
			Value:    userContext.ID,
			Path:     "/",
			MaxAge:   0,
			Secure:   !env.IsLocal(),
			HTTPOnly: false,
			SameSite: "Lax",
		})
	} else {
		// decrypt the encrypted enclave context
		decryptedJson, errDecrypt := crypt.Decrypt([]byte(c.Cookies(contextCookieName)))
		if errDecrypt != nil {
			return wipeContext(c)
		}
		// decrypt and unmarshal the enclave context cookie
		err = json.Unmarshal(decryptedJson, userContext)
		// ensure cookies match
		if err != nil || userContext.ID != c.Cookies(uidCookieName) {
			return wipeContext(c)
		}
	}

	// set user context for later handlers
	c.SetContext(userContext)

	return c.Next()
}

// wipeContext clears the context cookies and redirects the user home
func wipeContext(c fiber.Ctx) error {
	c.ClearCookie(contextCookieName)
	c.ClearCookie(uidCookieName)
	return c.Redirect().To("/")
}
