package middleware

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
)

// contentSecurityPolicy builds the site's CSP. It is deliberately permissive on
// inline scripts and styles because the view layer depends on them: inline
// onclick handlers (the theme/prefs buttons, the 404 back button), the no-flash
// theme <script> in layout.templ, and an inline style= on the theme swatch.
// Inline event handlers are covered ONLY by 'unsafe-inline' — a nonce or hash
// does not authorize them — so introducing a nonce scheme here would silently
// break those buttons. Every resource is locked to same-origin: howler (the one
// former third-party script) is now self-hosted. The single external host
// allowed is the self-hosted Plausible analytics instance (PLAUSIBLE_DOMAIN,
// kept out of VCS): script-src loads its tracker (layout.templ head) and
// connect-src lets that tracker POST pageview/event beacons to /api/event.
// With PLAUSIBLE_DOMAIN unset the policy is same-origin only.
//
// Tightening path: move the onclick handlers to addEventListener in lio.js,
// then swap script-src to 'self' plus a per-request nonce and drop
// 'unsafe-inline'.
func contentSecurityPolicy() string {
	plausible := ""
	if d := config.PlausibleDomain(); d != "" {
		plausible = " https://" + d
	}

	return "default-src 'self'; " +
		"script-src 'self' 'unsafe-inline'" + plausible + "; " +
		"style-src 'self' 'unsafe-inline'; " +
		"img-src 'self' data:; " +
		"font-src 'self'; " +
		"media-src 'self'; " +
		"connect-src 'self'" + plausible + "; " +
		"frame-ancestors 'none'; " +
		"base-uri 'self'; " +
		"form-action 'self'; " +
		"object-src 'none'"
}

// permissionsPolicy disables browser features the site never uses, so a future
// XSS or embedded resource can't reach for the camera, mic, location, or the
// Topics API on our behalf.
const permissionsPolicy = "camera=(), microphone=(), geolocation=(), browsing-topics=()"

// SecurityHeaders sets defensive response headers on every request. It is wired
// early (right after panic recovery) so the headers ride on error pages,
// redirects, and static responses alike.
func SecurityHeaders() fiber.Handler {
	csp := contentSecurityPolicy()

	return func(c fiber.Ctx) error {
		c.Set(fiber.HeaderXContentTypeOptions, "nosniff")
		c.Set(fiber.HeaderXFrameOptions, "DENY")
		c.Set(fiber.HeaderReferrerPolicy, "strict-origin-when-cross-origin")
		c.Set(fiber.HeaderPermissionsPolicy, permissionsPolicy)
		c.Set(fiber.HeaderContentSecurityPolicy, csp)

		// HSTS only in prod, where the browser-facing connection is always HTTPS
		// (TLS terminates at Cloudflare). Emitting it on the http dev server would
		// pin localhost to https and break local iteration.
		if env.IsProd() {
			c.Set(fiber.HeaderStrictTransportSecurity, "max-age=63072000; includeSubDomains")
		}

		return c.Next()
	}
}
