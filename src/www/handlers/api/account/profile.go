package account

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
)

// Logged-in profile edits (arch/ACCOUNTS_AUTH_RATINGS.md polish pass): the Edit
// Profile modal off the profile popover's pencil icon. Two low-stakes changes —
// setting/clearing the optional email, and the single allowed casing-only
// username change — plus a GET that prefills the modal. All require a live
// authenticated session (authed → 401), on top of the group rate limit and the
// stateless CSRF guard. These deliberately do NOT re-verify the password: email
// has no recovery value yet and a casing change is cosmetic (unlike the
// password / MFA management endpoints, which do re-verify).

// wireProfile attaches the profile-edit routes to the /api/auth group.
func wireProfile(g fiber.Router) {
	g.Get("/profile", ProfileHandler)
	g.Post("/email", EmailHandler)
	g.Post("/username", UsernameHandler)
}

// profileBody prefills the Edit Profile modal: the current username and email,
// and whether the one allowed casing-only username change is still available.
type profileBody struct {
	Username                string `json:"username"`
	Email                   string `json:"email"`
	UsernameChangeAvailable bool   `json:"usernameChangeAvailable"`
}

// ProfileHandler returns the logged-in account's editable profile fields. The
// email is not carried on the session (never rendered), so the modal fetches it
// here on open rather than paying a per-page-render DB read.
func ProfileHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	user, found, err := db.GetUserByID(*sess.UserID)
	if err != nil || !found {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not load profile"})
	}
	email := ""
	if user.Email != nil {
		email = *user.Email
	}
	return c.JSON(profileBody{
		Username:                user.Username,
		Email:                   email,
		UsernameChangeAvailable: !user.UsernameChanged,
	})
}

// EmailHandler sets, replaces, or clears the account email. An empty value
// clears it (there is no email verification/recovery yet, so this is a plain
// overwrite). Echoes back the stored value.
func EmailHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		Email string `json:"email"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	email, err := parseEmail(req.Email)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errBody{Error: err.Error()})
	}
	if err := db.UpdateEmail(*sess.UserID, email); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not update email"})
	}
	out := ""
	if email != nil {
		out = *email
	}
	return c.JSON(struct {
		Email string `json:"email"`
	}{Email: out})
}

// UsernameHandler applies the one allowed username change, restricted to
// altering the capitalization (the lowercased identity is immutable). It
// refuses a non-casing change, a no-op, and any second attempt. On success the
// session cache is dropped so the header re-renders the new casing on the
// client's follow-up reload.
func UsernameHandler(c fiber.Ctx) error {
	sess, ok := authed(c)
	if !ok {
		return nil
	}
	var req struct {
		Username string `json:"username"`
	}
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	newName := strings.TrimSpace(req.Username)
	if err := auth.ValidateUsername(newName); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errBody{Error: err.Error()})
	}

	user, found, err := db.GetUserByID(*sess.UserID)
	if err != nil || !found {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not update username"})
	}
	if user.UsernameChanged {
		return c.Status(fiber.StatusConflict).
			JSON(errBody{Error: "you've already used your one username change"})
	}
	// casing-only: the new name must be the same identity (case-insensitively)
	// and actually differ in case (a no-op would waste the one change).
	if !strings.EqualFold(newName, user.Username) {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "you can only change the capitalization of your username"})
	}
	if newName == user.Username {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "that's already your username"})
	}

	changed, err := db.UpdateUsernameCasing(*sess.UserID, newName)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(errBody{Error: "could not update username"})
	}
	if !changed {
		// lost the race: the one change was used between the read and the write
		return c.Status(fiber.StatusConflict).
			JSON(errBody{Error: "you've already used your one username change"})
	}

	// the session cache still holds the old display name for up to cacheTTL;
	// drop it so the next resolve (the client's reload) shows the new casing
	auth.DropSessionCache(c)
	return c.JSON(okBody{Username: newName})
}
