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

// authAPIMax is the per-client budget for the /api/auth group per window —
// register/login/logout plus the signup form's availability probe (which
// fires per keystroke, debounced client-side). The keyed per-username login
// limiter inside the auth package is the credential-stuffing defense; this
// one just bounds bulk abuse of the endpoints themselves.
const authAPIMax = 30

// authAPIWindow is the rolling window authAPIMax is measured over.
const authAPIWindow = time.Minute

// AuthAPILimiter rate-limits the /api/auth routes per client IP.
func AuthAPILimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          authAPIMax,
		Expiration:   authAPIWindow,
		KeyGenerator: clientIP,
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).
				JSON(fiber.Map{"error": "too many requests - slow down"})
		},
	})
}

// analysisMax is the per-client budget for the /api/analysis exploration
// endpoint per window. Each cache-missing request costs a real (budgeted)
// engine search, so this bounds a single client's CPU draw while staying
// generous for a human stepping through lines (~1-2 requests/second).
const analysisMax = 90

// analysisWindow is the rolling window analysisMax is measured over.
const analysisWindow = time.Minute

// AnalysisLimiter rate-limits the /api/analysis endpoint per client IP.
func AnalysisLimiter() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:          analysisMax,
		Expiration:   analysisWindow,
		KeyGenerator: clientIP,
		LimitReached: func(c fiber.Ctx) error {
			return c.Status(fiber.StatusTooManyRequests).
				JSON(fiber.Map{"error": "too many requests - slow down"})
		},
	})
}

// ClientIP exposes the resolved client address to handlers outside this
// package (the login rate limiter keys off it).
func ClientIP(c fiber.Ctx) string {
	return clientIP(c)
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
