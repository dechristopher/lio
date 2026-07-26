// Package report holds the one moderation surface ordinary players touch:
// filing a report about an opponent (arch/ADMIN_MODERATION.md Phase 4).
//
// It lives outside www/handlers/api/mod deliberately. Everything in that
// package requires a role and acts on someone; this requires only an account
// and merely asks a moderator to look. Keeping them apart means the privileged
// group's rate limit, gate and audit conventions never have to grow an
// exception for the one endpoint that is not privileged.
package report

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/user"
)

// maxNote bounds the free-text note. Long enough for "he moved instantly every
// move from a losing position", short enough that the queue stays scannable.
const maxNote = 1000

type errBody struct {
	Error string `json:"error"`
}

// Wire attaches the report endpoint to the given group.
func Wire(g fiber.Router) {
	g.Post("/", Handler)
}

// request is the report body. The target is named by *username* rather than by
// id: it is what the reporting player can see, it is stable across the pages
// that offer the control, and it saves threading account ids into the room
// payload purely so the client can hand one back.
type request struct {
	Username string `json:"username"`
	Category string `json:"category"`
	Note     string `json:"note"`
	// GameID is the game the report came out of, when it came out of one.
	GameID string `json:"gameId"`
}

// Handler files a report.
//
// The refusals are all about keeping the queue worth reading: only accounts can
// be reported (an anonymous opponent has nothing to sanction and a bot has no
// account), nobody can report themselves, and the reports_open_unique index
// stops one aggrieved player burying the queue in reports about one opponent.
// That last case answers 200, not an error — from the reporter's side "you have
// already told us" is a complete and true response, and telling them it failed
// would only invite them to try again.
func Handler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(errBody{Error: "reports are unavailable in this environment"})
	}
	acct := user.GetAccount(c)
	if acct == nil {
		// Reporting requires an account so the queue has someone to come back
		// to, and so a single anonymous visitor cannot file endlessly.
		return c.Status(fiber.StatusUnauthorized).
			JSON(errBody{Error: "log in to report a player"})
	}

	var req request
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}

	if !db.ValidCategory(req.Category) {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "pick a reason for the report"})
	}
	note := strings.TrimSpace(req.Note)
	if len(note) > maxNote {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "that note is too long"})
	}

	target, found, err := db.GetUserByUsername(strings.TrimSpace(req.Username))
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not file that report"})
	}
	if !found {
		return c.Status(fiber.StatusNotFound).
			JSON(errBody{Error: "no such account"})
	}
	if target.ID == acct.ID {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "you cannot report yourself"})
	}

	gameID := parseGameID(req.GameID)

	switch err := db.FileReport(acct.ID, target.ID, gameID, req.Category, note); {
	case err == nil:
		return c.SendStatus(fiber.StatusNoContent)
	case err == db.ErrAlreadyReported:
		return c.Status(fiber.StatusOK).
			JSON(fiber.Map{"already": true,
				"message": "You have already reported this player. A moderator will review it."})
	default:
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not file that report"})
	}
}

// parseGameID accepts the evidence link when it is a real uuid and drops it
// otherwise: a malformed id is not worth refusing a report over, and the FK
// would reject it anyway.
func parseGameID(s string) *uuid.UUID {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return nil
	}
	return &id
}
