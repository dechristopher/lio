package mod

import (
	"strconv"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/role"
)

// Working the report queue (arch/ADMIN_MODERATION.md Phase 4).
//
// Resolving a report is *not* the same as acting on the account it names —
// those are the player-page actions, each with its own audit entry. Closing a
// report only records that a moderator looked and decided, which is why "no
// action needed" is a first-class outcome here rather than something a
// moderator has to express by doing nothing and leaving the row open.

// resolveRequest is the queue's close action.
type resolveRequest struct {
	ID int64 `json:"id"`
	// Resolution is what the moderator decided. Required for the same reason
	// every other action needs a reason: the next moderator to look at this
	// account needs to know it was already considered, and what came of it.
	Resolution string `json:"resolution"`
}

// ResolveReportHandler closes one open report.
func ResolveReportHandler(c fiber.Ctx) error {
	sess, ok := actor(c, role.Mod)
	if !ok {
		return nil
	}

	var req resolveRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "malformed request"})
	}
	resolution, ok := reasonOf(c, req.Resolution)
	if !ok {
		return nil
	}

	targetID, closed, err := db.ResolveReport(req.ID, *sess.UserID, resolution)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not resolve that report"})
	}
	if !closed {
		// another moderator got there first; say so rather than silently
		// overwriting their decision
		return c.Status(fiber.StatusConflict).
			JSON(errBody{Error: "that report was already resolved"})
	}

	// Logged against the reported account, so the decision shows up in that
	// account's history alongside any sanction that followed it — a moderator
	// reading a player page can see both that a report was made and what was
	// concluded, without going to the queue.
	logAction(sess, targetID, "report", map[string]any{
		"report": strconv.FormatInt(req.ID, 10),
	}, resolution)

	return c.SendStatus(fiber.StatusNoContent)
}
