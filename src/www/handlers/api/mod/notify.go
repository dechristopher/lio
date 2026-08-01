package mod

import (
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/notify"
	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// The operator message composer on /system (arch/NOTIFICATIONS.md Phase 3):
// find one player, write to them.
//
// It is the third and last notification producer, and the only one a person
// composes by hand. The other two describe something that already happened — a
// moderation decision, a challenge — so their wording is the server's; this one
// carries whatever the operator typed, which is why it is length-bounded at both
// ends and written to the audit log.

const (
	// searchLimit bounds the player picker. It is a picker, not a directory:
	// somebody looking for one account types enough of the name to find it, and
	// a long list would only invite scrolling instead of typing.
	searchLimit = 8
	// searchMin is the shortest term worth answering. A single character matches
	// most of the site and tells the operator nothing.
	searchMin = 2

	// minNotifyBody rejects a message that says nothing. An operator message
	// arrives with the site's authority behind it, and "hi" spends that for
	// nothing.
	minNotifyBody = 8
	// maxNotifyBody bounds one message to what a notification row can show
	// without the panel becoming a document viewer.
	maxNotifyBody = 500
)

// userMatch is one hit in the picker.
type userMatch struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}

// notifyRequest is one composed message.
type notifyRequest struct {
	UserID int64  `json:"userId"`
	Body   string `json:"body"`
	// Choices makes the message an acknowledgement: it stays in the recipient's
	// bell, and in their badge, until they pick one. Empty for a message they
	// only have to read.
	//
	// The same flag a broadcast carries, and validated by the same function. A
	// question is worth asking of one player as well as of everybody — a
	// moderator answering a report can ask that player to confirm they have read
	// the decision — so it is a property of a notification rather than of a
	// broadcast.
	Choices []string `json:"choices"`
}

// SearchUsersHandler answers the player picker, closest match first.
func SearchUsersHandler(c fiber.Ctx) error {
	if _, ok := actor(c, role.Mod); !ok {
		return nil
	}
	// a live search is answered per keystroke and is account-specific; a cached
	// copy would show a stale list for a term the operator has moved past
	c.Set(fiber.HeaderCacheControl, "no-store")

	term := strings.TrimSpace(c.Query("q"))
	if len(term) < searchMin {
		// Not an error: this is what an empty box and the first keystroke look
		// like, and reporting them as failures would light the composer red
		// while somebody is still typing.
		return c.JSON(fiber.Map{"players": []userMatch{}})
	}

	rows, err := db.SearchUsers(term, searchLimit)
	if err != nil {
		util.Error(str.CDB, "user search failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not search players"})
	}
	players := make([]userMatch, 0, len(rows))
	for _, r := range rows {
		players = append(players, userMatch{ID: r.ID, Username: r.Username})
	}
	return c.JSON(fiber.Map{"players": players})
}

// NotifyUserHandler sends one operator message to one account.
//
// The recipient is resolved by id rather than trusted from the picker's label:
// the id is what the notification is written against, and confirming the account
// exists first means a mistyped or stale id refuses instead of writing a row
// pointing at nobody.
//
// No actor is recorded on the notification, for the same reason moderation
// notifications record none: the message speaks for the site, and naming the
// individual operator to the person they wrote to invites a reply nobody is
// watching for. The audit entry names them for other staff, which is where that
// belongs.
func NotifyUserHandler(c fiber.Ctx) error {
	sess, ok := actor(c, role.Mod)
	if !ok {
		return nil
	}

	var req notifyRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}

	body := strings.TrimSpace(req.Body)
	if len(body) < minNotifyBody {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "write a little more than that"})
	}
	if len(body) > maxNotifyBody {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "that message is too long"})
	}

	choices, err := cleanChoices(req.Choices)
	if err != nil {
		return c.Status(fiber.StatusUnprocessableEntity).JSON(errBody{Error: err.Error()})
	}

	rec, found, err := db.GetUserByID(req.UserID)
	if err != nil {
		util.Error(str.CDB, "notify target lookup failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not send that message"})
	}
	if !found {
		return c.Status(fiber.StatusNotFound).JSON(errBody{Error: "no such player"})
	}

	if err := notify.Push(db.NewNotification{
		UserID:  rec.ID,
		Kind:    db.KindSystem,
		Body:    body,
		Choices: choices,
	}, ""); err != nil {
		util.Error(str.CNotif, "operator notify failed target=%d error=%s", rec.ID, err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not send that message"})
	}

	// Audited like every other per-account action. The message itself is the
	// record — there is no separate reason to give, because what was sent is
	// exactly what needs to be reviewable.
	logAction(sess, rec.ID, "notify", map[string]any{}, body)

	return c.JSON(fiber.Map{"sent": rec.Username})
}
