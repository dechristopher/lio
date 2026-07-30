package mod

import (
	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/notify"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/view"
)

// Working the feedback inbox on /system.
//
// Deliberately the lightest surface in this package. Everything else here acts
// on a person and therefore demands a reason and writes a permanent audit
// entry; marking feedback read is neither — it is one moderator saying "seen",
// and asking them to justify that would train them to stop doing it, which
// would leave the unread badge permanently lit and useless.

// readRequest marks one submission read.
type readRequest struct {
	ID int64 `json:"id"`
}

// unreadResponse is the badge poll's answer. It carries the rendered label as
// well as the number so the dot a poll paints is worded identically to the one
// the server rendered — the pluralisation lives in one place
// (view.UnreadBadgeLabel), not in a second copy inside the client.
type unreadResponse struct {
	Unread int64  `json:"unread"`
	Label  string `json:"label"`
}

// UnreadFeedbackHandler reports how much feedback is still unread, so a page
// that is already open can light or clear its badge without being reloaded.
//
// Cheap by construction: db.UnreadFeedback serves a process-wide count cached
// for its own short TTL, so a poll is a mutex and a map read rather than a
// query, and polling faster than that TTL would only re-read the same number.
// That is what lets every moderator's open tab poll without the badge becoming
// a load source.
func UnreadFeedbackHandler(c fiber.Ctx) error {
	if _, ok := actor(c, role.Mod); !ok {
		return nil
	}
	// a poll's whole purpose is a fresh answer; letting it sit in any cache
	// would strand the badge on a number the page already had
	c.Set(fiber.HeaderCacheControl, "no-store")

	n := db.UnreadFeedback()
	return c.JSON(unreadResponse{Unread: n, Label: view.UnreadBadgeLabel(n)})
}

// ReadFeedbackHandler stamps one piece of feedback as read.
func ReadFeedbackHandler(c fiber.Ctx) error {
	sess, ok := actor(c, role.Mod)
	if !ok {
		return nil
	}

	var req readRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "malformed request"})
	}

	// The already-read case (ok=false) is not a conflict worth refusing over the
	// way a double-resolved report is: nothing anyone needs gets overwritten,
	// and the moderator's intent — this should not be in the unread count —
	// holds either way. Only a real failure is worth reporting.
	if _, err := db.MarkFeedbackRead(req.ID, *sess.UserID); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not mark that read"})
	}
	// The backlog is shared, so every moderator's badge just changed — including
	// this one's other tabs.
	notify.SendStaffCount()
	return c.SendStatus(fiber.StatusNoContent)
}

// ReadAllFeedbackHandler clears the whole unread backlog.
func ReadAllFeedbackHandler(c fiber.Ctx) error {
	sess, ok := actor(c, role.Mod)
	if !ok {
		return nil
	}
	if _, err := db.MarkAllFeedbackRead(*sess.UserID); err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not clear the inbox"})
	}
	notify.SendStaffCount()
	return c.SendStatus(fiber.StatusNoContent)
}
