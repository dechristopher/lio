package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/env"
)

// MutationGuard is a stateless, token-free CSRF defense for state-changing
// requests. Combined with SameSite=Lax identity cookies (which already withhold
// themselves from cross-site POSTs) it makes forged room-creation / join /
// cancel requests from other sites impossible without a per-request token.
//
// Safe methods (GET/HEAD/OPTIONS) pass through untouched — the WebSocket upgrade
// and every page/asset GET included. For a mutation it prefers the Fetch Metadata
// Sec-Fetch-Site header (sent by every evergreen browser): only a same-origin,
// same-site, or user-initiated (none) request is allowed; an explicit
// cross-site mutation is rejected. When the header is absent (older browsers,
// non-browser clients) it falls back to an Origin allowlist — the same list the
// WebSocket okOrigin check uses — and, as there, an absent Origin is allowed
// through because only a cross-site *browser* page is the threat this stops
// (curl et al. carry no ambient credentials to abuse).
func MutationGuard() fiber.Handler {
	return func(c fiber.Ctx) error {
		switch c.Method() {
		case fiber.MethodPost, fiber.MethodPut, fiber.MethodPatch, fiber.MethodDelete:
			// guarded below
		default:
			return c.Next()
		}

		// outside production the guard stands down entirely: LAN devices,
		// tunnels, and test harnesses reach non-prod servers from arbitrary
		// origins (which no static allowlist can anticipate), and the forged
		// cross-site mutation this stops only matters where real sessions
		// live. Mirrors ws.okOrigin and the wildcard config.CorsOrigins.
		if !env.IsProd() {
			return c.Next()
		}

		if site := c.Get(fiber.HeaderSecFetchSite); site != "" {
			switch site {
			case "same-origin", "same-site", "none":
				return c.Next()
			default: // "cross-site"
				return rejectCrossSite(c)
			}
		}

		// no Sec-Fetch-Site: fall back to Origin allowlist
		origin := c.Get(fiber.HeaderOrigin)
		if origin == "" {
			return c.Next()
		}
		for _, allowed := range strings.Split(config.CorsOrigins(), ",") {
			if origin == strings.TrimSpace(allowed) {
				return c.Next()
			}
		}
		return rejectCrossSite(c)
	}
}

func rejectCrossSite(c fiber.Ctx) error {
	return c.Status(fiber.StatusForbidden).SendString("cross-site request blocked")
}
