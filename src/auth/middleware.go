package auth

import (
	"strings"

	"github.com/gofiber/fiber/v3"
)

// legacy identity cookies from the pre-session era; actively expired once so
// returning visitors don't carry dead cookies around forever.
var legacyCookies = []string{"lio", "uid"}

// SessionMiddleware resolves (or mints) the request's session and attaches
// the identity to the fiber context. It replaced user.ContextMiddleware when
// the unified session system landed (arch/ACCOUNTS_AUTH_RATINGS.md).
func SessionMiddleware(c fiber.Ctx) error {
	path := c.Path()

	// WebSocket upgrades authenticate with the cookie they present, or not at
	// all: never mint a session for a socket and never touch cookies. iOS
	// Safari intermittently omits cookies from WS upgrade requests (see
	// ios-deploy-confirm-bug / webkit.org #255524), so a minted identity here
	// would seat the connection as a spectator whose game frames are silently
	// dropped. On any failure the request passes through with no context;
	// ws.connHandler rejects the empty-uid upgrade with close code 4001 the
	// client knows how to recover from.
	if strings.HasPrefix(path, "/socket") {
		if sess := FromRequest(c); sess != nil {
			c.SetContext(UserContext(sess))
		}
		return c.Next()
	}

	// Asset-shaped paths (a dotted final segment: hashed js/css, fonts,
	// sounds, piece art, manifest.json, OG card PNGs) never mint sessions —
	// a cookie-less asset fetch is a scraper or CDN probe, not a visitor,
	// and a session row per hit would be pure table bloat. Requests that do
	// carry a session simply pass through without resolving it (assets don't
	// need identity).
	if isAssetPath(path) {
		return c.Next()
	}

	sess := FromRequest(c)
	if sess == nil {
		// a fresh mint is the moment to shed any pre-session-era cookies —
		// do it before Mint writes the sid Set-Cookie so all cookie headers
		// land together (and even if Mint fails on a store error)
		expireLegacyCookies(c)
		sess = Mint(c)
		if sess == nil {
			// store failure (logged in Mint): serve the request without an
			// identity rather than failing the page
			return c.Next()
		}
	}

	c.SetContext(UserContext(sess))
	// surface the uid to the request logger (${locals:uid}); the old logger
	// format read the plaintext uid cookie, which no longer exists
	c.Locals("uid", sess.UID)

	return c.Next()
}

// isAssetPath reports whether the final path segment carries a file
// extension. Page and API routes are extension-less (room IDs are base58, no
// dots); everything the static handler serves — and the OG card PNGs — is
// dotted.
func isAssetPath(path string) bool {
	last := path
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		last = path[i+1:]
	}
	return strings.IndexByte(last, '.') >= 0
}

// expireLegacyCookies clears the retired lio/uid identity cookies when
// present.
func expireLegacyCookies(c fiber.Ctx) {
	for _, name := range legacyCookies {
		if c.Cookies(name) != "" {
			clearCookie(c, name)
		}
	}
}
