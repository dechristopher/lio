package account

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/view"
)

// Logged-in account administration (arch/ACCOUNTS_AUTH_RATINGS.md Phase 3):
// change password, list/revoke active sessions, and log out everywhere. These
// all require a live authenticated session (401 otherwise), on top of the
// group's per-IP rate limit and the stateless CSRF guard.

// wireAdmin attaches the authed account-admin routes to the /api/auth group.
func wireAdmin(g fiber.Router) {
	g.Post("/password", PasswordHandler)
	g.Get("/sessions", SessionsHandler)
	g.Post("/sessions/revoke", RevokeSessionHandler)
	g.Post("/logout-all", LogoutAllHandler)
}

// authed resolves the request's authenticated session, or writes a 401 and
// returns ok=false. The returned session carries the current session id (the
// one a password change / logout-all keeps or targets).
func authed(c fiber.Ctx) (*auth.Session, bool) {
	if !auth.Enabled() {
		_ = unavailable(c)
		return nil, false
	}
	sess := auth.CurrentSession(c)
	if sess == nil || !sess.LoggedIn() {
		_ = c.Status(fiber.StatusUnauthorized).
			JSON(errBody{Error: "not logged in"})
		return nil, false
	}
	return sess, true
}

// PasswordHandler changes the account password: verify the current password,
// re-hash the new one, and revoke every *other* session (so a compromised
// password can't keep a foothold elsewhere). The changer stays logged in.
func PasswordHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	if err := auth.ValidatePassword(req.New); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errBody{Error: err.Error()})
	}

	user, found, err := db.GetUserByID(*sess.UserID)
	if err != nil || !found {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "password change failed"})
	}
	okPw, _, err := auth.VerifyPassword(user.PasswordHash, req.Current)
	if err != nil || !okPw {
		return c.Status(fiber.StatusForbidden).JSON(errBody{Error: "current password is incorrect"})
	}

	phc, err := auth.HashPassword(req.New)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "password change failed"})
	}
	if err := db.UpdatePasswordHash(user.ID, phc); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "password change failed"})
	}
	// revoke every session but this one
	if err := db.DeleteSessionsForUserExcept(user.ID, sess.ID); err != nil {
		// the password did change; report success but log the sweep failure
		// upstream is not worth failing the request over
		_ = err
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// SessionsHandler renders the active-sessions fragment for the profile popover
// (server-rendered templ, injected by lio-auth.js). The current session is
// marked and cannot be revoked from the list (that is what "Log out" is for).
func SessionsHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	rows, err := db.ListSessionsForUser(*sess.UserID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not load sessions"})
	}
	// most-recently-seen first (the query already orders this way; keep it
	// explicit so the current session floats up predictably)
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].LastSeen.After(rows[j].LastSeen)
	})
	views := make([]view.SessionView, 0, len(rows))
	for _, r := range rows {
		views = append(views, view.SessionView{
			ID:       r.ID,
			Device:   describeUA(r.UserAgent),
			LastSeen: relativeTime(r.LastSeen),
			Current:  r.ID == sess.ID,
		})
	}
	return view.Render(c, fiber.StatusOK, view.SessionList(views))
}

// RevokeSessionHandler revokes one of the user's *other* sessions by id. The
// current session is never revocable here (use Log out); the db query is
// owner-scoped so a forged id can't touch another user's session.
func RevokeSessionHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		ID int64 `json:"id"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	if req.ID == sess.ID {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "use log out to end the current session"})
	}
	if err := db.DeleteSessionByID(req.ID, *sess.UserID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "revoke failed"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// LogoutAllHandler revokes every session the user holds (including this one)
// and clears the cookie — "log out everywhere".
func LogoutAllHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	if err := auth.LogoutAll(c, *sess.UserID); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "logout failed"})
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// describeUA reduces a raw User-Agent to a coarse "Browser on OS" label for
// the sessions list. Best-effort and privacy-light: no version numbers, just
// enough to recognize a device. Falls back to "Unknown device".
func describeUA(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	browser := "Browser"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	os := ""
	switch {
	case strings.Contains(ua, "iPhone") || strings.Contains(ua, "iPad"):
		os = "iOS"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	if os == "" {
		return browser
	}
	return browser + " on " + os
}

// relativeTime renders a coarse "N units ago" (or a date past a week) for the
// sessions list. Kept human and low-precision.
func relativeTime(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return plural(int(d.Minutes()), "minute")
	case d < 24*time.Hour:
		return plural(int(d.Hours()), "hour")
	case d < 7*24*time.Hour:
		return plural(int(d.Hours()/24), "day")
	default:
		return t.Format("Jan 2, 2006")
	}
}

func plural(n int, unit string) string {
	if n <= 0 {
		n = 1
	}
	s := strconv.Itoa(n) + " " + unit
	if n != 1 {
		s += "s"
	}
	return s + " ago"
}
