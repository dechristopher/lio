// Package feedback holds the player-facing "tell us what's wrong, or what's
// working" endpoint behind the prompt in the profile popover.
//
// It sits beside www/handlers/api/report rather than inside it: a report is an
// accusation about another account and carries a target, a duplicate guard and
// a moderator decision, while feedback is a message about the site with exactly
// one party and no outcome to report back. Keeping them apart means neither
// endpoint's validation has to ask which kind of thing it is holding.
//
// Nothing about the site is worth spamming a moderator's inbox over, so the
// abuse story is layered rather than resting on any one check: an account is
// required (identity and accountability), the group is rate-limited per client
// IP (burst), a rolling per-account cap bounds one person's total volume, a
// honeypot field catches the naive form-filling bot, and the body is length-
// bounded at both ends. None of it depends on the UI: the prompt only rendering
// for a logged-in viewer is cosmetic.
package feedback

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/notify"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
)

const (
	// minBody is the shortest submission worth storing. "ok" and an accidental
	// keypress are not feedback, and rejecting them here keeps the inbox worth
	// opening. Deliberately low — "clock is broken" is four words and perfectly
	// actionable.
	minBody = 8
	// maxBody bounds the message. Long enough for someone to actually describe
	// what happened, short enough that one submission cannot make the inbox
	// unreadable. The client caps the textarea at the same number.
	maxBody = 2000
	// maxPath bounds the captured page path. Real paths on this site are short
	// (a room id, a username); anything longer is not a path we generated.
	maxPath = 200

	// dailyCap is how many submissions one account may file inside capWindow.
	// Generous for a person who hits several things in one session, and a hard
	// stop on anyone treating the box as a chat window.
	dailyCap  = 10
	capWindow = 24 * time.Hour
)

type errBody struct {
	Error string `json:"error"`
}

// Wire attaches the feedback endpoint to the given group.
func Wire(g fiber.Router) {
	g.Post("/", Handler)
}

// request is the submission body.
type request struct {
	Kind string `json:"kind"`
	Body string `json:"body"`
	// Path is where the author was when they wrote it, captured client-side.
	// Context, not evidence — it is whatever the client said it was, which is
	// why it is validated into a same-origin path before anything renders it.
	Path string `json:"path"`
	// Website is a honeypot: it is hidden from people and has no meaning, so
	// anything in it came from something filling every field it found. See
	// Handler for why a filled one still answers success.
	Website string `json:"website"`
}

// Handler records one piece of feedback.
func Handler(c fiber.Ctx) error {
	if !auth.Enabled() {
		return c.Status(fiber.StatusServiceUnavailable).
			JSON(errBody{Error: "feedback is unavailable in this environment"})
	}
	acct := user.GetAccount(c)
	if acct == nil {
		return c.Status(fiber.StatusUnauthorized).
			JSON(errBody{Error: "log in to send feedback"})
	}

	var req request
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}

	// Honeypot: answer exactly as a successful submission would and store
	// nothing. Telling a bot which field gave it away only teaches it to skip
	// that field next time, and a human can never reach this branch — the input
	// is hidden and left empty.
	if strings.TrimSpace(req.Website) != "" {
		return c.SendStatus(fiber.StatusNoContent)
	}

	if !db.ValidFeedbackKind(req.Kind) {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "pick what kind of feedback this is"})
	}
	body := strings.TrimSpace(req.Body)
	if len(body) < minBody {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "tell us a little more than that"})
	}
	if len(body) > maxBody {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "that message is too long"})
	}

	// The rolling cap. Checked before the insert so the refusal is honest — the
	// alternative (insert, then notice) would store the row it just refused.
	sent, err := db.RecentFeedbackByUser(acct.ID, capWindow)
	if err != nil {
		util.Error(str.CDB, "feedback cap check failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not send that feedback"})
	}
	if sent >= dailyCap {
		return c.Status(fiber.StatusTooManyRequests).
			JSON(errBody{Error: "you've sent a lot of feedback today — try again tomorrow"})
	}

	if err := db.SubmitFeedback(acct.ID, req.Kind, body, safePath(req.Path)); err != nil {
		util.Error(str.CDB, "feedback submit failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not send that feedback"})
	}
	// Light every moderator's badge now (arch/NOTIFICATIONS.md). Their pages are
	// open and are not going to be reloaded, and the count on a socket is only
	// read at connect — so without this push, feedback would sit unseen until
	// somebody happened to navigate.
	notify.SendStaffCount()
	return c.SendStatus(fiber.StatusNoContent)
}

// safePath narrows the client-reported page to something that can only ever be
// a link back into this site, dropping it entirely otherwise.
//
// The inbox renders it as a link so a moderator can go look at the page being
// described, which means an unchecked value here would be an arbitrary
// attacker-chosen href on a privileged page. Requiring a single leading slash
// rejects both absolute URLs ("https://…") and protocol-relative ones ("//…"),
// leaving only same-origin paths; the query string and fragment are dropped
// because neither helps and both are somewhere to hide a payload.
func safePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" || len(p) > maxPath {
		return ""
	}
	if !strings.HasPrefix(p, "/") || strings.HasPrefix(p, "//") {
		return ""
	}
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	// control characters have no business in a path and would render oddly in
	// the inbox
	if strings.ContainsFunc(p, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
		return ""
	}
	return p
}
