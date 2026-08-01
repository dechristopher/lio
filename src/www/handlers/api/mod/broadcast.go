package mod

import (
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/notify"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// The broadcast composer on /system (arch/NOTIFICATIONS.md, *A broadcast is one
// row*): write one message, and every account reads it in their own bell.
//
// Admin-only, unlike the one-player composer beside it. That is the line
// /system already draws: writing to one account is moderator work, and anything
// every visitor sees — the site notice, maintenance, ratings — is an admin's.
// A broadcast reaches every account at once, so it belongs on the same side as
// the switches.
//
// Logged with a NULL target, like a settings change. The audit log records
// everything privileged, not only what was done to one person, and the message
// itself is the payload — what was said is exactly what needs to be reviewable.

const (
	// minBroadcastBody rejects a message that says nothing. It goes to every
	// account on the site with the site's authority behind it, and "hi" spends
	// that for nothing.
	minBroadcastBody = 8
	// maxBroadcastBody bounds one message to what a notification row can show
	// without the panel becoming a document viewer. The same bound the
	// one-player composer uses.
	maxBroadcastBody = 500
	// maxBroadcastChoices bounds the options on a question. Beyond a handful the
	// row is a form, and a form belongs on a page rather than in a dropdown from
	// the header.
	maxBroadcastChoices = 4
	// maxChoiceLength bounds one option's label. These render as buttons in a
	// 20rem panel; a sentence would wrap the row into a paragraph.
	maxChoiceLength = 24
	// broadcastsShown bounds the sent list on /system. Recent history, not an
	// archive: what an operator needs is what is live and what just ended.
	broadcastsShown = 20
	// maxBroadcastDays bounds a scheduled expiry. Beyond this "until I retire
	// it" is the honest setting, and a date a year out is indistinguishable from
	// one nobody chose.
	maxBroadcastDays = 90
)

// broadcastRequest is one composed broadcast.
type broadcastRequest struct {
	Body string `json:"body"`
	Link string `json:"link"`
	// Choices makes the message an acknowledgement: it stays in every reader's
	// bell until they pick one. Empty for an ordinary announcement.
	Choices []string `json:"choices"`
	// ExpiresDays retires the message automatically after this many days. 0
	// leaves it running until somebody retires it by hand.
	ExpiresDays int `json:"expiresDays"`
	// Reason is the audit entry's justification, required like every other
	// privileged change.
	Reason string `json:"reason"`
}

// retireRequest pulls one message.
type retireRequest struct {
	ID     int64  `json:"id"`
	Reason string `json:"reason"`
}

// BroadcastHandler sends one message to every account.
func BroadcastHandler(c fiber.Ctx) error {
	sess, ok := actor(c, role.Admin)
	if !ok {
		return nil
	}

	var req broadcastRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	reason, ok := reasonOf(c, req.Reason)
	if !ok {
		return nil
	}

	body := strings.TrimSpace(req.Body)
	if len(body) < minBroadcastBody {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "write a little more than that"})
	}
	if len(body) > maxBroadcastBody {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "that message is too long"})
	}

	// A path on this site, never a full URL. The row is rendered as a link in
	// everybody's panel, and a broadcast that could point off-site would be the
	// most trusted phishing surface the site has.
	link := strings.TrimSpace(req.Link)
	if link != "" && !strings.HasPrefix(link, "/") {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "the link must be a path on this site, starting with /"})
	}
	if strings.HasPrefix(link, "//") {
		// "//evil.example" is a path to a browser and another origin to a person.
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "that link leaves the site"})
	}

	choices, err := cleanChoices(req.Choices)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errBody{Error: err.Error()})
	}

	var expires time.Time
	if req.ExpiresDays > 0 {
		if req.ExpiresDays > maxBroadcastDays {
			return c.Status(fiber.StatusUnprocessableEntity).
				JSON(errBody{Error: "pick a shorter run, or leave it open"})
		}
		expires = time.Now().Add(time.Duration(req.ExpiresDays) * 24 * time.Hour)
	}

	row, err := notify.Broadcast(db.NewBroadcast{
		ActorID: sess.UserID,
		Body:    body,
		Link:    link,
		Choices: choices,
		Expires: expires,
	})
	if err != nil {
		util.Error(str.CNotif, "broadcast failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not send that broadcast"})
	}

	detail := map[string]any{"body": body}
	if len(choices) > 0 {
		detail["asks"] = strings.Join(choices, " / ")
	}
	if !expires.IsZero() {
		detail["until"] = expires.UTC().Format(time.RFC3339)
	}
	if err := db.LogModAction(*sess.UserID, nil, "broadcast", detail, reason); err != nil {
		util.Error(str.CDB, "broadcast log failed error=%s", err.Error())
	}

	return c.JSON(fiber.Map{"id": row.ID})
}

// RetireBroadcastHandler pulls a message: it stops showing in every panel from
// now. Answers already given are kept — the tally is the record of what was
// asked and what came back, and retiring the question does not unask it.
func RetireBroadcastHandler(c fiber.Ctx) error {
	sess, ok := actor(c, role.Admin)
	if !ok {
		return nil
	}

	var req retireRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	reason, ok := reasonOf(c, req.Reason)
	if !ok {
		return nil
	}

	retired, err := db.RetireBroadcast(req.ID)
	if err != nil {
		util.Error(str.CNotif, "broadcast retire failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not retire that broadcast"})
	}
	if !retired {
		// Already ended, or never existed. Both mean the same thing to the
		// operator: it is not showing.
		return c.Status(fiber.StatusConflict).
			JSON(errBody{Error: "that broadcast is not running"})
	}

	if err := db.LogModAction(*sess.UserID, nil, "broadcast",
		map[string]any{"retired": req.ID}, reason); err != nil {
		util.Error(str.CDB, "broadcast retire log failed error=%s", err.Error())
	}
	return c.JSON(fiber.Map{"retired": req.ID})
}

// cleanChoices normalizes and validates the options on a question.
//
// It trims, drops empties and refuses duplicates. A duplicate matters more than
// it looks: the answer is stored as its own label, so two options reading "Yes"
// would tally as one and the operator could never tell which button was pressed.
func cleanChoices(in []string) ([]string, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		choice := strings.TrimSpace(raw)
		if choice == "" {
			continue
		}
		if len(choice) > maxChoiceLength {
			return nil, errChoiceTooLong
		}
		key := strings.ToLower(choice)
		if _, dup := seen[key]; dup {
			return nil, errChoiceDuplicate
		}
		seen[key] = struct{}{}
		out = append(out, choice)
	}
	if len(out) == 0 {
		// Every option was blank. An acknowledgement with no answer could never
		// be cleared, so this is a message that asks nothing.
		return nil, nil
	}
	if len(out) > maxBroadcastChoices {
		return nil, errTooManyChoices
	}
	return out, nil
}

// The three ways a set of options is refused, as values rather than formatted
// strings: each is shown to the operator as-is.
var (
	errChoiceTooLong   = broadcastError("an option should be a word or two, not a sentence")
	errChoiceDuplicate = broadcastError("two options read the same, so the answers could not be told apart")
	errTooManyChoices  = broadcastError("that is too many options for a notification row")
)

// broadcastError is a plain string error, so the message the operator reads is
// the message written here.
type broadcastError string

func (e broadcastError) Error() string { return string(e) }
