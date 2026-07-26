package mod

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/settings"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// Runtime site controls (arch/ADMIN_MODERATION.md Phase 3). Admin-only — these
// affect everyone at once, which is a different order of blast radius from a
// per-account sanction, so they sit above the moderator ladder.
//
// Each change is logged to mod_actions with a NULL target: the audit log is the
// record of *everything* privileged, not only what was done to people.

// maxNoticeLength bounds the banner. It renders on every page, so a runaway
// value would be a site-wide layout incident, and templ escapes but does not
// truncate.
const maxNoticeLength = 300

// settingsRequest is the site-controls form body. Every field is optional; a
// request changes only the switches it names, so the form can post one control
// at a time without having to restate the rest of the site's configuration.
type settingsRequest struct {
	Reason           *string `json:"reason"`
	NoticeText       *string `json:"noticeText"`
	NoticeLevel      *string `json:"noticeLevel"`
	RegistrationOpen *bool   `json:"registrationOpen"`
	RatedEnabled     *bool   `json:"ratedEnabled"`
	Maintenance      *bool   `json:"maintenance"`
}

// SettingsHandler returns the current snapshot, for the /mod console's form.
func SettingsHandler(c fiber.Ctx) error {
	if _, ok := actor(c, role.Admin); !ok {
		return nil
	}
	return c.Status(fiber.StatusOK).JSON(settings.Current())
}

// UpdateSettingsHandler applies the named switches.
//
// Writes go through db and then Invalidate, so the admin's next request already
// reflects the change instead of waiting out the snapshot TTL. Other instances
// pick it up within that TTL — the deliberate tradeoff for having no pubsub.
func UpdateSettingsHandler(c fiber.Ctx) error {
	sess, ok := actor(c, role.Admin)
	if !ok {
		return nil
	}

	var req settingsRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "malformed request"})
	}

	reason := ""
	if req.Reason != nil {
		reason = strings.TrimSpace(*req.Reason)
	}
	reason, ok = reasonOf(c, reason)
	if !ok {
		return nil
	}

	// What actually changed, for the audit entry — built as the writes happen so
	// the log records the request as applied, not as submitted. Each switch
	// records its previous value alongside the new one: "maintenance=true" says
	// what the site became, but only "maintenance off → on" says whether this
	// entry is the one that started an incident.
	before := settings.Current()
	changed := map[string]any{}

	if req.NoticeText != nil {
		text := strings.TrimSpace(*req.NoticeText)
		if len(text) > maxNoticeLength {
			return c.Status(fiber.StatusUnprocessableEntity).
				JSON(errBody{Error: "that notice is too long"})
		}
		// an empty banner is the absence of one, so clear the override rather
		// than storing "" — see db.ClearSetting
		if err := writeOrClear(settings.KeyNoticeText, text, text == "",
			*sess.UserID); err != nil {
			return settingsError(c)
		}
		changed["notice"] = orNone(text)
		changed["noticeWas"] = orNone(before.NoticeText)

		// Clearing the banner clears its styling too. A level with no text is
		// not a state the site has — nothing renders it — so leaving the row
		// behind would only mean the next notice quietly inherits a styling
		// choice made for an unrelated one.
		if text == "" {
			if err := db.ClearSetting(settings.KeyNoticeLevel); err != nil {
				return settingsError(c)
			}
			req.NoticeLevel = nil
		}
	}

	if req.NoticeLevel != nil {
		level := settings.LevelInfo
		if *req.NoticeLevel == settings.LevelWarn {
			level = settings.LevelWarn
		}
		if err := db.SetSetting(settings.KeyNoticeLevel, level, *sess.UserID); err != nil {
			return settingsError(c)
		}
		changed["noticeLevel"] = level
	}

	for _, f := range []struct {
		key   string
		value *bool
		was   bool
		label string
	}{
		{settings.KeyRegistration, req.RegistrationOpen, before.RegistrationOpen, "registrationOpen"},
		{settings.KeyRated, req.RatedEnabled, before.RatedEnabled, "ratedEnabled"},
		{settings.KeyMaintenance, req.Maintenance, before.Maintenance, "maintenance"},
	} {
		if f.value == nil {
			continue
		}
		if err := db.SetSetting(f.key, settings.Flag(*f.value), *sess.UserID); err != nil {
			return settingsError(c)
		}
		changed[f.label] = *f.value
		changed[f.label+"Was"] = f.was
	}

	if len(changed) == 0 {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "nothing to change"})
	}

	settings.Invalidate()

	// a site-level action has no target account
	if err := db.LogModAction(*sess.UserID, nil, "setting", changed, reason); err != nil {
		util.Error(str.CDB, "settings audit log failed error=%s", err.Error())
	}

	return c.Status(fiber.StatusOK).JSON(settings.Current())
}

// writeOrClear stores a value, or removes the override when clearing back to
// the default.
func writeOrClear(key, value string, clear bool, actorID int64) error {
	if clear {
		return db.ClearSetting(key)
	}
	return db.SetSetting(key, value, actorID)
}

// settingsError reports a failed write without leaking Postgres detail.
func settingsError(c fiber.Ctx) error {
	return c.Status(fiber.StatusInternalServerError).
		JSON(errBody{Error: "could not apply that change"})
}
