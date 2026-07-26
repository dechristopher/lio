package mod

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// Force-closing a live room from the /system ops view (arch/ADMIN_MODERATION.md
// Phase 5).
//
// Admin-only. It is the bluntest tool in the console — it ends something two
// people may be in the middle of — and unlike a ban it is aimed at a room
// rather than an account, so the ladder rules that protect moderators from each
// other do not apply to it. Restricting it to admins is the substitute.

type closeRoomRequest struct {
	RoomID string `json:"roomId"`
	Reason string `json:"reason"`
}

// CloseRoomHandler ends one live room.
//
// The audit entry has no target account: a room is not a person, and inventing
// one of its players as the "target" would put an operational cleanup in
// somebody's moderation history as though it were about them.
func CloseRoomHandler(c fiber.Ctx) error {
	sess, ok := actor(c, role.Admin)
	if !ok {
		return nil
	}

	var req closeRoomRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "malformed request"})
	}
	reason, ok := reasonOf(c, req.Reason)
	if !ok {
		return nil
	}
	roomID := strings.TrimSpace(req.RoomID)
	if roomID == "" {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "which room?"})
	}

	if !room.CloseRoom(roomID) {
		// already gone, or in a state with nothing left to end. Not an error
		// worth alarming anyone about — the operator's goal (this room is not
		// running) already holds.
		return c.Status(fiber.StatusConflict).
			JSON(errBody{Error: "that room is no longer live"})
	}

	if err := db.LogModAction(*sess.UserID, nil, "room", map[string]any{
		"room": roomID,
	}, reason); err != nil {
		util.Error(str.CDB, "room close audit log failed room=%s error=%s",
			roomID, err.Error())
	}

	return c.SendStatus(fiber.StatusNoContent)
}
