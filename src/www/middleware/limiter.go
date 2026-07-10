package middleware

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

// roomCreateMax is the per-client room-creation budget per window. Creating a
// room spins up goroutines, a clock, and (for bot games) real engine CPU, so
// this bounds a single client's ability to exhaust those. It is generous for a
// human clicking around and only bites automated abuse.
const roomCreateMax = 20

// roomCreateWindow is the rolling window roomCreateMax is measured over.
const roomCreateWindow = time.Minute

// RoomCreateLimiter rate-limits the /new/* room-creation routes per client IP.
func RoomCreateLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          roomCreateMax,
		Expiration:   roomCreateWindow,
		KeyGenerator: clientIP,
		LimitReached: func(c fiber.Ctx) error {
			// these are navigations; bounce home rather than show a raw 429 page
			return c.Redirect().To("/")
		},
	})
}

// clientIP resolves the real client address behind the Cloudflare tunnel. The
// app's only ingress is cloudflared over loopback, so fiber's own c.IP() sees
// 127.0.0.1 for every request — useless as a rate-limit key. Cloudflare sets
// CF-Connecting-IP to the authenticated client IP (a client-supplied value can't
// override it at the edge), so prefer that, then the first X-Forwarded-For hop,
// then c.IP() for local/dev where neither header is present.
func clientIP(c fiber.Ctx) string {
	if ip := c.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if xff := c.Get(fiber.HeaderXForwardedFor); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	return c.IP()
}
