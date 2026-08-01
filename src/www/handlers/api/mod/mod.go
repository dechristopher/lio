// Package mod holds the privileged site administration and moderation API
// (arch/ADMIN_MODERATION.md): sanctions, title and role assignment, forced
// renames, site controls and the audit feed.
//
// Not to be confused with www/handlers/api/account/admin.go, which is
// *self-service account* administration (a user changing their own password
// and revoking their own sessions) and requires no privilege beyond being
// logged in.
//
// Authorization here is always the handler's check, never the absence of a UI
// control: the mod bar not rendering for an ordinary visitor is cosmetic, and
// every route below independently resolves the caller's role.
package mod

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// errBody is the uniform JSON error envelope, matching the account API's.
type errBody struct {
	Error string `json:"error"`
}

// Wire attaches the moderation routes. The caller supplies the group (already
// rate-limited); every handler re-checks privilege, so the grouping is
// organizational rather than a security boundary.
func Wire(g fiber.Router) {
	// audit feed, readable by any moderator
	g.Get("/actions", ActionsHandler)

	// per-account actions, posted by the mod bar on a player page
	g.Post("/ban", BanHandler)
	g.Post("/unban", UnbanHandler)
	g.Post("/title", TitleHandler)
	g.Post("/role", RoleHandler)
	g.Post("/rename", RenameHandler)

	// report queue (see reports.go)
	g.Post("/report/resolve", ResolveReportHandler)

	// feedback inbox (see feedback.go). The unread count is a GET because it is
	// polled by every moderator's open page to keep the badge current.
	g.Get("/feedback/unread", UnreadFeedbackHandler)
	g.Post("/feedback/read", ReadFeedbackHandler)
	g.Post("/feedback/read-all", ReadAllFeedbackHandler)

	// the operator message composer on /system (see notify.go): find a player,
	// write to them
	g.Get("/users/search", SearchUsersHandler)
	g.Post("/notify", NotifyUserHandler)

	// the broadcast composer on /system (admin-only; see broadcast.go): one
	// message, every account
	g.Post("/broadcast", BroadcastHandler)
	g.Post("/broadcast/retire", RetireBroadcastHandler)

	// live ops: force-close a room (admin-only; see rooms.go)
	g.Post("/room/close", CloseRoomHandler)

	// runtime site controls (admin-only; see settings.go)
	g.Get("/settings", SettingsHandler)
	g.Post("/settings", UpdateSettingsHandler)
}

// actor is the resolved caller of a moderation route: their session plus the
// role they hold. It writes the 401/403 itself and returns ok=false when the
// caller may not moderate at all.
//
// The role is read from the session (resolved per request, cached ≤30s), which
// is also what a demotion has to invalidate — auth.DropUserSessions is called
// on role changes for exactly that reason.
func actor(c fiber.Ctx, need role.Role) (*auth.Session, bool) {
	if !auth.Enabled() {
		_ = c.Status(fiber.StatusServiceUnavailable).
			JSON(errBody{Error: "accounts are unavailable in this environment"})
		return nil, false
	}
	sess := auth.CurrentSession(c)
	if sess == nil || !sess.LoggedIn() {
		_ = c.Status(fiber.StatusUnauthorized).JSON(errBody{Error: "not logged in"})
		return nil, false
	}
	if !sess.Role.AtLeast(need) {
		// deliberately identical to what a logged-in non-moderator sees for a
		// route that does not exist: privilege boundaries are not an oracle
		_ = c.Status(fiber.StatusNotFound).JSON(errBody{Error: "not found"})
		return nil, false
	}
	return sess, true
}

// modOnly gates a route on the moderator ladder (mod or admin).
func modOnly(c fiber.Ctx) (*auth.Session, bool) { return actor(c, role.Mod) }

// adminOnly gates a route on admin — role changes and site controls.
//
//nolint:unused // used by the role/settings handlers as those phases land
func adminOnly(c fiber.Ctx) (*auth.Session, bool) { return actor(c, role.Admin) }

// target resolves the account a moderation action names and enforces who may
// act on whom. Writes the error response and returns ok=false on refusal.
//
// The matrix:
//
//	actor  → player   mod    other admin                      self
//	mod       yes      no     no                               no
//	admin     yes      yes    only if the actor granted it     yes, minus ban/role
//
// Two rules beyond the plain ladder:
//
//   - **Admins may administer their own account.** Harmless edits (title,
//     rename) are the point; ban and role are refused per-action below, so an
//     admin can neither lock themselves out nor demote themselves into a site
//     with one fewer admin than its operator believes.
//   - **An admin may act on a peer admin only if they granted that admin
//     role**, per db.AdminGrantor. This is broader than "cannot demote": an
//     admin who could ban a peer could remove them anyway, so the gate covers
//     every action rather than leaving a route around itself. An admin
//     promoted outside the app (the SQL bootstrap) has no grantor on record
//     and is therefore untouchable through the UI — deliberately, since that
//     account is the recovery path when the UI is what has gone wrong.
func target(c fiber.Ctx, sess *auth.Session, userID int64) (db.UserRecord, bool) {
	rec, found, err := db.GetUserByID(userID)
	if err != nil {
		_ = c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not load that account"})
		return db.UserRecord{}, false
	}
	if !found {
		_ = c.Status(fiber.StatusNotFound).JSON(errBody{Error: "no such account"})
		return db.UserRecord{}, false
	}

	if isSelf(sess, rec.ID) {
		if !sess.Role.CanAdmin() {
			_ = c.Status(fiber.StatusForbidden).
				JSON(errBody{Error: "you cannot moderate your own account"})
			return db.UserRecord{}, false
		}
		return rec, true
	}

	if !sess.Role.CanActOn(rec.Role) {
		_ = c.Status(fiber.StatusForbidden).
			JSON(errBody{Error: "that account outranks you"})
		return db.UserRecord{}, false
	}
	if rec.Role.CanAdmin() && !grantedBy(sess, rec.ID) {
		_ = c.Status(fiber.StatusForbidden).
			JSON(errBody{Error: "only the admin who promoted this account can act on it"})
		return db.UserRecord{}, false
	}
	return rec, true
}

// isSelf reports whether the session owns the given account.
func isSelf(sess *auth.Session, userID int64) bool {
	return sess.UserID != nil && *sess.UserID == userID
}

// grantedBy reports whether this session's account is the one on record as
// having promoted userID to admin. A lookup failure denies: an authorization
// question that cannot be answered is not an approval.
func grantedBy(sess *auth.Session, userID int64) bool {
	grantor, ok, err := db.AdminGrantor(userID)
	if err != nil {
		util.Error(str.CDB, "admin grantor lookup failed user=%d error=%s",
			userID, err.Error())
		return false
	}
	return ok && sess.UserID != nil && grantor == *sess.UserID
}

// selfRefused writes the refusal for an action an admin may not take on their
// own account. Ban and role are the two: both would let one admin quietly
// reduce the site's operator coverage, and neither has a legitimate
// self-service use.
func selfRefused(c fiber.Ctx, what string) error {
	return c.Status(fiber.StatusForbidden).
		JSON(errBody{Error: "you cannot " + what + " your own account"})
}

// reasonOf validates the mandatory justification attached to every action. It
// is stored in the audit log and read by other moderators, so an empty one is
// refused rather than silently defaulted.
func reasonOf(c fiber.Ctx, reason string) (string, bool) {
	const maxReason = 500
	// Trimmed here, not just in the browser: the API is the boundary, and a
	// reason of three spaces satisfies "not empty" while telling a reviewer
	// nothing. The stored value is the trimmed one for the same reason.
	reason = strings.TrimSpace(reason)
	if reason == "" {
		_ = c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "a reason is required"})
		return "", false
	}
	if len(reason) > maxReason {
		_ = c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "that reason is too long"})
		return "", false
	}
	return reason, true
}

// ActionsHandler serves the global audit feed as JSON, newest first, honouring
// the same filters the /system page uses. Every moderator sees every other
// moderator's actions: there is no silent moderation, and the log is the
// accountability model for the tools themselves.
func ActionsHandler(c fiber.Ctx) error {
	if _, ok := modOnly(c); !ok {
		return nil
	}
	const pageSize = 100
	filter := db.ModActionFilter{
		Action: c.Query("action"),
		Query:  c.Query("q"),
	}
	actions, err := db.ListModActions(filter, pageSize, 0)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not load the audit log"})
	}
	return c.Status(fiber.StatusOK).JSON(actions)
}
