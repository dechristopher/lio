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
	// are silently dropped, and a wipe-and-redirect would break the handshake and
	// trap the client in a reconnect loop. On any failure we simply pass the
	// request through with no context; ws.connHandler rejects the empty-uid
	// upgrade with a dedicated close code the client knows how to recover from.
	if strings.HasPrefix(c.Path(), "/socket") {
		if userContext, ok := decodeContext(c, c.Cookies(contextCookieName)); ok {
			c.SetContext(userContext)
		}
		return c.Next()
	}

	// Resolve the identity from the enclave cookie. A missing cookie, one we
	// can't decrypt or validate (tampered, or produced by the retired
	// unauthenticated CFB scheme — GCM's Open rejects it), all land the same
	// way: mint a fresh anonymous identity in place. Minting overwrites both
	// cookies, so there is no separate wipe or redirect and the current request
	// proceeds under a valid identity instead of bouncing the visitor home.
	userContext, ok := decodeContext(c, c.Cookies(contextCookieName))
	if !ok {
		userContext, ok = mintAnonymous(c)
		if !ok {
			// an unresolvable crypto key lands here: every visitor stays
			// cookie-less and every WS upgrade is identity-less (a 4001
			// close loop), so scream instead of failing silently
			return c.Next()
		}
	}

	// set user context for later handlers
	c.SetContext(userContext)

	return c.Next()
}

// decodeContext decrypts and validates the enclave cookie, returning the user
// context it encodes. ok is false when there is no cookie, when it fails to
// decrypt (tampered, or produced by the retired CFB scheme) or unmarshal, or
// when its embedded ID does not match the plaintext uid cookie — the tie that
// stops a mixed/forged cookie pair from being trusted.
func decodeContext(c fiber.Ctx, enclave string) (*Context, bool) {
	if enclave == "" {
		return nil, false
	}

	decryptedJson, err := crypt.Decrypt([]byte(enclave))
	if err != nil {
		return nil, false
	}

	userContext := new(Context)
	if json.Unmarshal(decryptedJson, userContext) != nil {
		return nil, false
	}

	if userContext.ID != c.Cookies(uidCookieName) {
		return nil, false
	}

	return userContext, true
}

// mintAnonymous generates a fresh anonymous identity and writes both the
// encrypted enclave cookie and the plaintext uid cookie. ok is false only when
// the identity cannot be marshaled (an unresolvable crypto key), in which case
// the caller serves the request without a context.
func mintAnonymous(c fiber.Ctx) (*Context, bool) {
	// generate new anonymous context
	userContext := Anonymous()

	// marshal anonymous context to encrypted JSON
	contextCookie, err := userContext.MarshalJSON()
	if err != nil {
		util.Error(str.CUser, str.EIdentityMint, err.Error())
		return nil, false
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

	return userContext, true
}
