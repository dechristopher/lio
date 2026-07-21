package account

import (
	"errors"
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

// parseEmail normalizes and lightly validates an optional email, shared by
// registration and the profile email edit. Empty → nil (unset / cleared).
// There is no email infrastructure yet, so this is a shape check, not a
// deliverability check.
func parseEmail(raw string) (*string, error) {
	e := strings.TrimSpace(raw)
	if e == "" {
		return nil, nil
	}
	if len(e) > 254 || strings.Count(e, "@") != 1 ||
		strings.HasPrefix(e, "@") || strings.HasSuffix(e, "@") {
		return nil, errors.New("that email address doesn't look right")
	}
	return &e, nil
}

// Wire attaches the auth API handlers to the given /api/auth router group.
func Wire(g fiber.Router) {
	g.Post("/register", RegisterHandler)
	g.Post("/login", LoginHandler)
	g.Post("/logout", LogoutHandler)
	g.Get("/username-available", UsernameAvailableHandler)
	g.Get("/ratings", RatingsHandler)

	// logged-in account administration (password / sessions / logout-all)
	wireAdmin(g)

	// logged-in profile edits (email + one-time casing-only username change)
	wireProfile(g)

	// MFA: login-time second factor + management (arch Phase 4)
	wireMFA(g)
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
	email, err := parseEmail(req.Email)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: err.Error()})
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

	// second factor (arch/ACCOUNTS_AUTH_RATINGS.md Phase 4): password is only
	// the first factor here. If the account has TOTP or a passkey enrolled, the
	// visitor is NOT logged in yet — issue a short-lived pending token and let
	// them complete a factor via the /login/{totp,recovery,webauthn/*} routes.
	if methods, hasMFA := loginMFAMethods(rec.ID, rec.TOTPConfirmed); hasMFA {
		return c.Status(fiber.StatusOK).JSON(mfaChallengeBody{
			MFA:     true,
			Pending: auth.NewPending(rec.ID, rec.Username),
			Methods: methods,
		})
	}

	if err := auth.Login(c, auth.FromRequest(c), rec.ID, rec.Username); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "login failed"})
	}
	return c.Status(fiber.StatusOK).JSON(okBody{Username: rec.Username})
}

// loginMFAMethods reports which second factors an account offers (and whether it
// has any). recovery is only advertised alongside a real factor — a
// recovery-only account can't exist.
func loginMFAMethods(userID int64, totpConfirmed bool) (mfaMethodsBody, bool) {
	passkeys, _ := db.CountWebAuthnCredentials(userID)
	recovery, _ := db.CountUnusedRecoveryCodes(userID)
	m := mfaMethodsBody{
		TOTP:     totpConfirmed,
		Passkey:  passkeys > 0,
		Recovery: recovery > 0,
	}
	return m, m.TOTP || m.Passkey
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
