package mod

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/notify"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// The per-account moderation actions posted by the mod bar on a player page.
//
// Every handler here follows the same four beats, in this order:
//  1. resolve the actor and their privilege (modOnly / adminOnly)
//  2. resolve the target and the ladder rules (target: no self-action, no
//     acting on a peer or an admin)
//  3. require a reason
//  4. apply, then write the audit entry
//
// The audit entry is written after the change succeeds, so the log never claims
// something that did not happen. A failure to log is reported but does not roll
// the action back: an unlogged ban is bad, an un-banned cheat is worse.

// banRequest is the shared request body. Not every field applies to every
// action; each handler reads only what it needs.
type banRequest struct {
	UserID   int64  `json:"userId"`
	Reason   string `json:"reason"`
	Duration string `json:"duration"` // "24h" / "168h" / "permanent"
	TitleID  string `json:"titleId"`  // "" clears the title
	Role     string `json:"role"`
	Username string `json:"username"`
}

// bind reads and validates the shared body, resolving the actor, the target and
// the reason in one step — the preamble every action below needs.
func bind(c fiber.Ctx, need role.Role) (*auth.Session, db.UserRecord, banRequest, bool) {
	sess, ok := actor(c, need)
	if !ok {
		return nil, db.UserRecord{}, banRequest{}, false
	}
	var req banRequest
	if err := c.Bind().Body(&req); err != nil {
		_ = c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
		return nil, db.UserRecord{}, banRequest{}, false
	}
	rec, ok := target(c, sess, req.UserID)
	if !ok {
		return nil, db.UserRecord{}, banRequest{}, false
	}
	reason, ok := reasonOf(c, req.Reason)
	if !ok {
		return nil, db.UserRecord{}, banRequest{}, false
	}
	req.Reason = reason
	return sess, rec, req, true
}

// logAction records the action, reporting (but not failing on) a log error.
func logAction(sess *auth.Session, targetID int64, action string,
	detail map[string]any, reason string) {
	if err := db.LogModAction(*sess.UserID, &targetID, action, detail, reason); err != nil {
		util.Error(str.CDB, "mod action log failed action=%s target=%d error=%s",
			action, targetID, err.Error())
	}
}

// notifyTarget tells the account what was done to it (arch/NOTIFICATIONS.md).
// It runs after the audit entry, and only for the actions a player has to know
// about: a changed username, a granted or removed title, a lifted ban. A ban
// itself is not among them, because the ban screen at the next login says more
// than a bell can.
//
// The acting moderator is deliberately not recorded as the actor. The audit feed
// names them for other staff, which is where that belongs; naming them to the
// person they sanctioned invites retaliation and tells the player nothing they
// can act on. The row therefore carries no actor and renders with no link to
// one.
//
// A failure here is logged and swallowed, exactly like a failed audit write: the
// action already happened, and refusing to report success would invite the
// moderator to repeat it.
func notifyTarget(targetID int64, body, link string) {
	if err := notify.Push(db.NewNotification{
		UserID: targetID,
		Kind:   db.KindModAction,
		Body:   body,
		Link:   link,
	}, ""); err != nil {
		util.Error(str.CNotif, "mod notification failed target=%d error=%s",
			targetID, err.Error())
	}
}

// BanHandler sanctions an account: record the ban, sign it out everywhere, and
// end any game it is currently playing as a forfeit.
//
// The order matters and is the whole enforcement story (arch/ADMIN_MODERATION.md):
// the row is written first so nothing can re-authenticate, then the sessions are
// deleted and their cached copies evicted — without the eviction a resolved
// session stays valid for up to the cache TTL — and only then is the live game
// ended, so the forfeited player cannot simply keep playing.
func BanHandler(c fiber.Ctx) error {
	sess, rec, req, ok := bind(c, role.Mod)
	if !ok {
		return nil
	}
	// an admin may administer their own account, but not lock themselves out
	if isSelf(sess, rec.ID) {
		return selfRefused(c, "ban")
	}

	until, permanent, err := parseDuration(req.Duration)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: err.Error()})
	}

	if err := db.BanUser(rec.ID, until, req.Reason); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not apply the ban"})
	}

	if err := db.DeleteSessionsForUser(rec.ID); err != nil {
		util.Error(str.CAuth, "ban session revoke failed user=%d error=%s",
			rec.ID, err.Error())
	}
	auth.DropUserSessions(rec.ID)
	forfeited := room.ForfeitUser(rec.ID)

	// Logged after the enforcement rather than before it so the entry can record
	// what the ban actually did — how many live rooms it ended is the detail
	// someone reviewing a disputed sanction asks about first, and it is
	// unknowable until the sweep has run.
	detail := map[string]any{
		"permanent": permanent,
		"duration":  durationLabel(req.Duration),
		"forfeited": forfeited,
	}
	if !permanent {
		detail["until"] = until.UTC().Format(time.RFC3339)
	}
	logAction(sess, rec.ID, "ban", detail, req.Reason)

	return c.SendStatus(fiber.StatusNoContent)
}

// UnbanHandler lifts a sanction early. Nothing is restored beyond the ability
// to log in: the forfeited game stays forfeited, which is why a ban is worth
// getting right the first time.
func UnbanHandler(c fiber.Ctx) error {
	sess, rec, req, ok := bind(c, role.Mod)
	if !ok {
		return nil
	}
	// capture what is being lifted before it is gone: an unban entry that only
	// says "unbanned" leaves a reviewer unable to tell a 24-hour timeout being
	// cut short from a permanent ban being reversed
	lifted := "not banned"
	if rec.Ban.Banned {
		lifted = banTermLabel(rec.Ban)
	}

	if err := db.UnbanUser(rec.ID); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not lift the ban"})
	}
	logAction(sess, rec.ID, "unban", map[string]any{
		"lifted":    lifted,
		"banReason": rec.Ban.Reason,
	}, req.Reason)
	notifyTarget(rec.ID, "Your account has been restored. You can play again.", "")
	return c.SendStatus(fiber.StatusNoContent)
}

// TitleHandler assigns or clears an account's display title. An empty titleId
// clears it — the same control does both, since "remove this title" is as much
// a moderation action as granting one and deserves the same audit entry.
func TitleHandler(c fiber.Ctx) error {
	sess, rec, req, ok := bind(c, role.Mod)
	if !ok {
		return nil
	}

	var titleID *int16
	if req.TitleID != "" {
		id, err := strconv.ParseInt(req.TitleID, 10, 16)
		if err != nil {
			return c.Status(fiber.StatusUnprocessableEntity).
				JSON(errBody{Error: "unknown title"})
		}
		id16 := int16(id)
		titleID = &id16
	}

	if err := db.SetUserTitle(rec.ID, titleID); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not set the title"})
	}
	// the account's own sessions cache the title for display; drop them so the
	// badge updates on their next request rather than up to a cache TTL later
	auth.DropUserSessions(rec.ID)

	// Log the title's *code*, not the row id the form submitted: "to=3" is
	// meaningless in a feed a human reads. Re-reading the row is also the only
	// way to record what was actually stored rather than what was asked for.
	assigned := ""
	if updated, found, err := db.GetUserByID(rec.ID); err == nil && found {
		assigned = updated.Title.Code
	}
	logAction(sess, rec.ID, "title", map[string]any{
		"from": orNone(rec.Title.Code),
		"to":   orNone(assigned),
	}, req.Reason)
	if assigned != "" {
		notifyTarget(rec.ID, "You have been given the title "+assigned+".",
			"/@/"+rec.Username)
	} else if rec.Title.Code != "" {
		notifyTarget(rec.ID, "Your title has been removed.", "/@/"+rec.Username)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// RoleHandler appoints or demotes. Admin-only, and guarded against removing the
// last admin — an instance with no admin cannot appoint one back through the UI
// and would need a hand-written SQL statement to recover.
func RoleHandler(c fiber.Ctx) error {
	sess, rec, req, ok := bind(c, role.Admin)
	if !ok {
		return nil
	}
	// self-demotion is refused for the same reason the last-admin guard exists:
	// it silently reduces the site's operator coverage, and an admin who wants
	// out should be stood down by another admin who then knows it happened
	if isSelf(sess, rec.ID) {
		return selfRefused(c, "change the role of")
	}

	next := role.Parse(req.Role)
	if next.String() != req.Role {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "unknown role"})
	}
	if rec.Role.CanAdmin() && !next.CanAdmin() {
		admins, err := db.CountAdmins()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).
				JSON(errBody{Error: "could not verify admin count"})
		}
		if admins <= 1 {
			return c.Status(fiber.StatusConflict).
				JSON(errBody{Error: "that is the last admin — appoint another first"})
		}
	}

	if err := db.SetUserRole(rec.ID, next); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not set the role"})
	}
	// a demotion must bite immediately: the old role is cached on their live
	// sessions and would otherwise keep authorizing moderation for a cache TTL
	auth.DropUserSessions(rec.ID)

	logAction(sess, rec.ID, "role", map[string]any{
		"from": rec.Role.String(),
		"to":   next.String(),
	}, req.Reason)
	if body, link := roleNotice(rec.Role, next); body != "" {
		notifyTarget(rec.ID, body, link)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// roleNotice writes what an account is told about its own role changing, and
// returns an empty body when there is nothing to say.
//
// An appointment is the one change here the account cannot discover any other
// way: nothing on screen says "you may now moderate", and the tools simply
// appear the next time they look. It links to /system, because the first useful
// thing a new moderator does is open the page they just gained.
//
// A demotion is announced for the same reason a removed title is: the controls
// disappear, and finding that out by pressing one is worse than being told. It
// carries no link — the page it would point at is the one they no longer have.
//
// Like every other message from this file it names no moderator. The audit feed
// records who made the appointment, which is where staff read it.
func roleNotice(from, to role.Role) (body, link string) {
	if from.String() == to.String() {
		return "", ""
	}
	switch {
	case to.CanAdmin():
		return "You are now an administrator.", "/system"
	case to.CanModerate():
		return "You are now a moderator.", "/system"
	case from.CanAdmin():
		return "Your administrator access has been removed.", ""
	case from.CanModerate():
		return "Your moderator access has been removed.", ""
	}
	return "", ""
}

// RenameHandler applies a forced rename — the proportionate sanction for a
// username that beat the registration filter, where a ban would be excessive.
// The new name is held to the same policy a registration is, and the rename
// deliberately does not consume the player's own one-time rename allowance.
func RenameHandler(c fiber.Ctx) error {
	sess, rec, req, ok := bind(c, role.Mod)
	if !ok {
		return nil
	}

	if err := auth.ValidateUsername(req.Username); err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: err.Error()})
	}
	if err := db.ForceRename(rec.ID, req.Username); err != nil {
		if err == db.ErrUsernameTaken {
			return c.Status(fiber.StatusConflict).
				JSON(errBody{Error: "that username is taken"})
		}
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not rename the account"})
	}
	// their sessions carry the old display name
	auth.DropUserSessions(rec.ID)

	logAction(sess, rec.ID, "rename", map[string]any{
		"from": rec.Username,
		"to":   req.Username,
	}, req.Reason)
	// The one action here the player cannot discover any other way: their name
	// simply changes under them, and every link to their old one stops working.
	notifyTarget(rec.ID, "Your username has been changed to "+req.Username+".",
		"/@/"+req.Username)
	return c.Status(fiber.StatusOK).JSON(fiber.Map{"username": req.Username})
}

// durationLabel names a ban term the way the picker offered it, so the audit
// entry reads in the same units the moderator chose rather than as an interval
// the reader has to convert.
func durationLabel(d string) string {
	switch d {
	case "24h":
		return "24 hours"
	case "168h":
		return "7 days"
	case "720h":
		return "30 days"
	case "permanent":
		return "permanent"
	}
	return d
}

// banTermLabel describes a sanction that is being lifted.
func banTermLabel(b db.BanState) string {
	if b.Permanent {
		return "permanent"
	}
	return "until " + b.Until.UTC().Format(time.RFC3339)
}

// orNone renders an empty value as an explicit "none", so a log entry never
// shows a blank where a value belongs and leaves the reader guessing whether
// it was absent or the field failed to record.
func orNone(s string) string {
	if s == "" {
		return "none"
	}
	return s
}

// parseDuration resolves a ban term from the picker. Anything not on the list
// is refused rather than defaulted: a mistyped duration silently becoming a
// permanent ban would be the worst possible failure mode.
func parseDuration(s string) (until time.Time, permanent bool, err error) {
	if s == "permanent" {
		return time.Time{}, true, nil
	}
	switch s {
	case "24h", "168h", "720h":
		d, _ := time.ParseDuration(s)
		return time.Now().Add(d), false, nil
	}
	return time.Time{}, false, errBadDuration
}

// errBadDuration is returned for a term outside the offered set.
var errBadDuration = &durationError{}

type durationError struct{}

func (e *durationError) Error() string { return "unknown ban duration" }
