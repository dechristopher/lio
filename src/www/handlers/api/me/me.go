// Package me holds the endpoints an account uses to read and change its own
// state: the notification panel behind the bell in the header
// (arch/NOTIFICATIONS.md), and the display preferences the header's popover
// offers (the prefs package).
//
// Everything here is scoped to the session. No handler accepts an account id
// from a client, and no handler takes a username in a path: the answer is
// always "the account that made this request". That is what keeps the group
// safe without a role check — the endpoints are reachable by any signed-in
// visitor, and each one can only ever reach that visitor's own rows.
//
// The badge itself does not come from here. It arrives on the socket, on every
// connect and on every new message. This package serves the list, which loads
// on the first open of the panel, and the two writes that mark rows read.
package me

import (
	"sort"
	"strings"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/auth"
	"github.com/dechristopher/lio/db"
	"github.com/dechristopher/lio/notify"
	"github.com/dechristopher/lio/room"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/user"
	"github.com/dechristopher/lio/util"
	"github.com/dechristopher/lio/www/ws/proto"
)

// panelLimit bounds the list. The panel is a menu, not an archive: a person
// reads the newest few and marks the rest read. Thirty fills the scroll area
// several times over.
const panelLimit = 30

// noticeDeclined is the home-page notice key a declined challenger is sent
// home with. The copy for it lives in IndexHandler, which is the only thing
// that reads the key — this is the one place that writes it.
const noticeDeclined = "challenge-declined"

type errBody struct {
	Error string `json:"error"`
}

// Wire attaches the account's own endpoints to the given group.
func Wire(g fiber.Router) {
	g.Get("/notifications", ListHandler)
	g.Post("/notifications/read", ReadHandler)
	g.Post("/notifications/read-all", ReadAllHandler)
	g.Post("/notifications/answer", AnswerHandler)
	g.Post("/challenge/decline", DeclineChallengeHandler)
	g.Post("/prefs", PrefHandler)
}

// listResponse is the panel's payload. The items use the socket's own item
// shape (proto.NotifyItem) rather than a second one: the client renders rows
// from this list and from the arrival frame with one function, and two shapes
// would let those two paths drift apart.
type listResponse struct {
	Unread int64              `json:"unread"`
	Items  []proto.NotifyItem `json:"items"`
}

// readRequest marks one row read. Broadcast picks which store the id belongs
// to: the two have separate sequences, so the flag is part of the address and
// not a hint.
type readRequest struct {
	ID        int64 `json:"id"`
	Broadcast bool  `json:"bc"`
}

// answerRequest answers a message that demands an answer. Choice is validated
// against the message's own options in SQL, so nothing here has to hold a list
// of what is acceptable.
type answerRequest struct {
	ID        int64  `json:"id"`
	Broadcast bool   `json:"bc"`
	Choice    string `json:"choice"`
}

// declineRequest turns down a direct challenge. Room is what the refusal acts
// on; ID is the notification to retire alongside it, and may be 0.
type declineRequest struct {
	Room string `json:"room"`
	ID   int64  `json:"id"`
}

// account resolves the signed-in account, or writes the refusal and reports
// false. Everything in this group belongs to an account — its notifications,
// its preferences — so an anonymous visitor has nothing to reach here.
func account(c fiber.Ctx) (*user.Account, bool) {
	if !auth.Enabled() {
		_ = c.Status(fiber.StatusServiceUnavailable).
			JSON(errBody{Error: "accounts are unavailable in this environment"})
		return nil, false
	}
	acct := user.GetAccount(c)
	if acct == nil {
		_ = c.Status(fiber.StatusUnauthorized).
			JSON(errBody{Error: "log in first"})
		return nil, false
	}
	return acct, true
}

// ListHandler returns the account's notifications, newest first.
func ListHandler(c fiber.Ctx) error {
	acct, ok := account(c)
	if !ok {
		return nil
	}
	// One account's private list. It must never sit in a shared cache, and a
	// panel that reopens must not read a copy from before the last message.
	c.Set(fiber.HeaderCacheControl, "no-store")

	rows, err := db.ListNotifications(acct.ID, panelLimit)
	if err != nil {
		util.Error(str.CNotif, "notification list failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not load notifications"})
	}

	// notify.Item is the single mapping from a stored row to the wire shape,
	// shared with the socket path so the panel's list and a message arriving
	// live can never describe the same notification differently.
	follows := followedActors(acct.ID, rows)
	items := make([]proto.NotifyItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, notify.Item(r, follows))
	}

	// The broadcasts this account should see, merged into the same list. They
	// come from a different table and cost nothing to ask for while nothing is
	// being broadcast, but the reader has one panel: two sections, one for
	// "messages" and one for "announcements", would make them look in two places
	// for the same thing.
	//
	// A failed read leaves the notifications alone rather than failing the
	// panel. The badge would then disagree with the list until the next open,
	// which is a smaller wrong than an empty panel.
	if live, bErr := db.BroadcastsFor(acct.ID, panelLimit); bErr != nil {
		util.Error(str.CNotif, "broadcast list failed error=%s", bErr.Error())
	} else {
		for _, b := range live {
			items = append(items, notify.BroadcastItem(b))
		}
		// Newest first across both sources. The client keeps whatever order it
		// is given for everything except the rows that need an answer, which it
		// floats to the top itself.
		sort.SliceStable(items, func(i, j int) bool {
			return items[i].Created > items[j].Created
		})
	}

	return c.JSON(listResponse{Unread: notify.Unread(acct.ID), Items: items})
}

// followedActors resolves, for the whole page at once, which of the accounts
// that followed this reader the reader already follows back. That is what
// decides the state of the toggle on a follow row.
//
// One indexed probe bounded by the panel's own limit, not one per row: the
// batched read is the same one every other follow list on the site makes
// (arch/FOLLOWING.md), and the ids are gathered from the follow rows only, so a
// panel holding none costs nothing at all.
//
// A failed read reports "not following", which paints every toggle in its
// default state. The write path is idempotent, so a button that started on the
// wrong label still reaches the right state when it is pressed.
func followedActors(viewerID int64, rows []db.Notification) notify.FollowLookup {
	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		if r.Kind == db.KindFollow && r.ActorID != 0 {
			ids = append(ids, r.ActorID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	followed, err := db.FollowedAmong(viewerID, ids)
	if err != nil {
		util.Error(str.CDB, "notification follow state failed error=%s", err.Error())
		return nil
	}
	return func(actorID int64) bool {
		_, ok := followed[actorID]
		return ok
	}
}

// ReadHandler marks one row read.
//
// The row must belong to the session's account. That test is in the query
// itself (MarkNotificationRead is scoped by user_id), so a guessed id from
// another account changes nothing and reports nothing.
//
// A broadcast is read differently, because it is one row shared by everybody
// and cannot carry one reader's state: reading it moves this account's
// watermark up to that message instead. The move is monotonic, so an old
// message read after a new one cannot bring the new one back.
func ReadHandler(c fiber.Ctx) error {
	acct, ok := account(c)
	if !ok {
		return nil
	}

	var req readRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).
			JSON(errBody{Error: "malformed request"})
	}

	if req.Broadcast {
		if err := db.SeeBroadcast(acct.ID, req.ID); err != nil {
			util.Error(str.CNotif, "broadcast read failed error=%s", err.Error())
			return c.Status(fiber.StatusInternalServerError).
				JSON(errBody{Error: "could not mark that read"})
		}
		return countAfterWrite(c, acct)
	}

	// A row that was already read, and a row this account does not own, are the
	// same answer to the caller: nothing changed. Only a real failure is worth
	// reporting — and telling a caller which of the two it was would report
	// whether some other account's row exists.
	if _, err := db.MarkNotificationRead(req.ID, acct.ID); err != nil {
		util.Error(str.CNotif, "notification read failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not mark that read"})
	}
	return countAfterWrite(c, acct)
}

// ReadAllHandler clears the account's unread backlog: its own messages, and
// every announcement sent up to now.
//
// Neither write finishes something that asks a question — not a live challenge,
// not an unanswered acknowledgement, and not an unanswered broadcast. Opening
// the bell means "I have seen these", which is the whole of what most rows
// need, and none of what those need.
func ReadAllHandler(c fiber.Ctx) error {
	acct, ok := account(c)
	if !ok {
		return nil
	}
	if _, err := db.MarkAllNotificationsRead(acct.ID); err != nil {
		util.Error(str.CNotif, "notification read-all failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not clear notifications"})
	}
	// Best effort, and deliberately not a failure: the account's own backlog is
	// already cleared, and refusing the whole request over the watermark would
	// undo nothing and report an error for work that was done.
	if err := db.MarkBroadcastsSeen(acct.ID); err != nil {
		util.Error(str.CNotif, "broadcast read-all failed error=%s", err.Error())
	}
	return countAfterWrite(c, acct)
}

// AnswerHandler records the reader's answer to a message that demands one
// (arch/NOTIFICATIONS.md, *A message can ask a question*).
//
// The choice is checked against the message's own options inside the SQL, for
// both stores. So this handler holds no list of acceptable answers, and a
// crafted request cannot store an option the sender never offered — it simply
// matches no row.
//
// A write that changed nothing is reported as a refusal rather than as success.
// Every way to get there means the question is not open to this reader: they do
// not own the row, it asks nothing, it has ended, or they already answered. The
// client repaints from the count either way, so a stale panel corrects itself.
func AnswerHandler(c fiber.Ctx) error {
	acct, ok := account(c)
	if !ok {
		return nil
	}

	var req answerRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}
	choice := strings.TrimSpace(req.Choice)
	if choice == "" {
		return c.Status(fiber.StatusUnprocessableEntity).
			JSON(errBody{Error: "pick one of the options"})
	}

	var (
		answered bool
		err      error
	)
	if req.Broadcast {
		answered, err = db.AnswerBroadcast(req.ID, acct.ID, choice)
	} else {
		answered, err = db.AnswerNotification(req.ID, acct.ID, choice)
	}
	if err != nil {
		util.Error(str.CNotif, "notification answer failed error=%s", err.Error())
		return c.Status(fiber.StatusInternalServerError).
			JSON(errBody{Error: "could not record that"})
	}
	if !answered {
		return c.Status(fiber.StatusConflict).
			JSON(errBody{Error: "that is no longer open"})
	}
	return countAfterWrite(c, acct)
}

// DeclineChallengeHandler turns down a direct challenge addressed to this
// account (arch/NOTIFICATIONS.md Phase 2).
//
// Accept has no endpoint — it is the room link, and the ordinary join path. A
// refusal needs one because it has to reach the challenger, who is sitting on
// the waiting page: closing the room is what tells them, through the redirect
// their page already listens for.
//
// The room is the authority on who may refuse, not the notification: a row id
// is guessable and a link is shareable, while room.IsInvited is the same test
// that decides who may take the seat.
func DeclineChallengeHandler(c fiber.Ctx) error {
	acct, ok := account(c)
	if !ok {
		return nil
	}

	var req declineRequest
	if err := c.Bind().Body(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(errBody{Error: "malformed request"})
	}

	// Retire the notification either way. A challenge whose room has already
	// gone — the challenger left, or it expired — is still something the
	// recipient has now dealt with, and leaving it unread would keep a dead
	// invitation lit in their panel.
	if req.ID != 0 {
		if _, err := db.MarkNotificationRead(req.ID, acct.ID); err != nil {
			util.Error(str.CNotif, "challenge decline read failed error=%s", err.Error())
		}
	}

	instance, err := room.Get(req.Room)
	if err != nil || instance == nil {
		// already gone: the refusal has nothing left to do, and saying so would
		// only report a race the person cannot act on
		return countAfterWrite(c, acct)
	}
	if !instance.IsInvited(&acct.ID) {
		return c.Status(fiber.StatusForbidden).
			JSON(errBody{Error: "that challenge is not yours to decline"})
	}

	// Tell the challenger before the room goes: they are sitting on the waiting
	// page, and closing the room underneath them would otherwise send them home
	// with the generic "that room is gone" — which reads like a fault rather
	// than an answer. Ordered before Cancel because cancelling tears down the
	// channel this writes to.
	//
	// The notice key is read by IndexHandler, which owns the copy shown for it.
	instance.RedirectWaiting("/?notice=" + noticeDeclined)

	// Cancel refuses once the room is past waiting for players, which is the
	// race where the challenger cancelled or somebody already joined. Nothing to
	// report: the invitation is over either way.
	instance.Cancel()
	return countAfterWrite(c, acct)
}

// countAfterWrite answers with the new unread count and pushes that same count
// to the account's other connections.
//
// Both halves are necessary. The tab that made the request learns the count
// from this response. Every other tab and device the person has open holds its
// own badge, and nothing else would ever tell them the backlog is gone.
func countAfterWrite(c fiber.Ctx, acct *user.Account) error {
	c.Set(fiber.HeaderCacheControl, "no-store")
	notify.SendCount(acct.ID, acct.Role.CanModerate())
	return c.JSON(fiber.Map{"unread": notify.Unread(acct.ID)})
}
