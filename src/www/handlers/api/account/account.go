package account

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/www/middleware"
)

// The /api/auth handlers (arch/ACCOUNTS_AUTH_RATINGS.md Phase 1): register,
// login, logout, and the signup form's username-availability probe. All JSON
// in/out, driven by lio-auth.js. CSRF cover is the stateless MutationGuard +
// SameSite=Lax session cookie; no token layer.

// errBody is the uniform JSON error envelope.
type errBody struct {
	Error string `json:"error"`
}

// okBody acknowledges success with the (display-case) username.
type okBody struct {
	Username string `json:"username"`
}

// credentials is the register/login request body. Email is optional and only
// read at registration.
type credentials struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Email    string `json:"email"`
}

// unavailable is the uniform 503 for PG-less local dev (prod refuses to boot
// without Postgres, so accounts are always enabled there).
func unavailable(c fiber.Ctx) error {
	return c.Status(fiber.StatusServiceUnavailable).
		JSON(errBody{Error: "accounts are unavailable"})
}

// Wire attaches the auth API handlers to the given /api/auth router group.
func Wire(g fiber.Router) {
	g.Post("/register", RegisterHandler)
	g.Post("/login", LoginHandler)
	g.Post("/logout", LogoutHandler)
	g.Get("/username-available", UsernameAvailableHandler)
}

// RegisterHandler creates an account and logs the visitor in by upgrading
// their current session in place.
func RegisterHandler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return unavailable(c)
	}

	var req credentials
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "malformed request"})
	}

	username := strings.TrimSpace(req.Username)
	if err := auth.ValidateUsername(username); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: err.Error()})
	}
	if err := auth.ValidatePassword(req.Password); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: err.Error()})
	}
	var email *string
	if e := strings.TrimSpace(req.Email); e != "" {
		if len(e) > 254 || strings.Count(e, "@") != 1 ||
			strings.HasPrefix(e, "@") || strings.HasSuffix(e, "@") {
			return c.Status(fiber.StatusUnprocessableEntity).
				JSON(errBody{Error: "that email address doesn't look right"})
		}
		email = &e
	}

	phc, err := auth.HashPassword(req.Password)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "registration failed"})
	}

	id, err := db.CreateUser(username, email, phc)
	if err == db.ErrUsernameTaken {
		return c.Status(fiber.StatusConflict).
			JSON(errBody{Error: "that username is taken"})
	}
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "registration failed"})
	}

	if err := auth.Login(c, auth.FromRequest(c), id, username); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "registration succeeded but login failed - try logging in"})
	}
	return c.Status(fiber.StatusOK).JSON(okBody{Username: username})
}

// LoginHandler verifies credentials and upgrades the visitor's session.
// Unknown usernames burn a dummy verification so response timing does not
// reveal which usernames exist, and the response never distinguishes which
// credential was wrong.
func LoginHandler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return unavailable(c)
	}

	var req credentials
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "malformed request"})
	}
	username := strings.TrimSpace(req.Username)

	if !auth.AllowLogin(middleware.ClientIP(c) + "|" + strings.ToLower(username)) {
		return c.Status(fiber.StatusTooManyRequests).
			JSON(errBody{Error: "too many attempts - wait a few minutes"})
	}

	failed := func() error {
		return c.Status(fiber.StatusUnauthorized).
			JSON(errBody{Error: "invalid username or password"})
	}

	rec, found, err := db.GetUserByUsername(username)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "login failed"})
	}
	if !found {
		auth.VerifyDummy(req.Password)
		return failed()
	}

	ok, needsRehash, err := auth.VerifyPassword(rec.PasswordHash, req.Password)
	if err != nil || !ok {
		return failed()
	}

	// stored hash predates the current Argon2 params: transparently re-hash
	// with the plaintext we hold right now
	if needsRehash {
		if phc, err := auth.HashPassword(req.Password); err == nil {
			_ = db.UpdatePasswordHash(rec.ID, phc)
		}
	}

	if err := auth.Login(c, auth.FromRequest(c), rec.ID, rec.Username); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "login failed"})
	}
	return c.Status(fiber.StatusOK).JSON(okBody{Username: rec.Username})
}

// LogoutHandler revokes the current session. Works even when accounts are
// disabled (there is still an anonymous session to shed) and for anonymous
// sessions (harmless: the next page load mints a fresh one).
func LogoutHandler(c fiber.Ctx) error {
	auth.Logout(c)
	return c.SendStatus(fiber.StatusNoContent)
}

// UsernameAvailableHandler answers the signup form's live availability probe.
func UsernameAvailableHandler(c fiber.Ctx) error {
	type availBody struct {
		Available bool   `json:"available"`
		Reason    string `json:"reason,omitempty"`
	}
	if !auth.Enabled() {
		return unavailable(c)
	}
	u := strings.TrimSpace(c.Query("u"))
	if err := auth.ValidateUsername(u); err != nil {
		return c.JSON(availBody{Available: false, Reason: err.Error()})
	}
	taken, err := db.UsernameTaken(u)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "availability check failed"})
	}
	if taken {
		return c.JSON(availBody{Available: false, Reason: "that username is taken"})
	}
	return c.JSON(availBody{Available: true})
}
