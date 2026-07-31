// Package me holds the endpoints an account uses to read its own state. Today
// that is the notification panel behind the bell in the header
// (arch/NOTIFICATIONS.md).
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
	g.Post("/challenge/decline", DeclineChallengeHandler)
}

// listResponse is the panel's payload. The items use the socket's own item
// shape (proto.NotifyItem) rather than a second one: the client renders rows
// from this list and from the arrival frame with one function, and two shapes
// would let those two paths drift apart.
type listResponse struct {
	Unread int64              `json:"unread"`
	Items  []proto.NotifyItem `json:"items"`
}

// readRequest marks one row read.
type readRequest struct {
	ID int64 `json:"id"`
}

// declineRequest turns down a direct challenge. Room is what the refusal acts
// on; ID is the notification to retire alongside it, and may be 0.
type declineRequest struct {
	Room string `json:"room"`
	ID   int64  `json:"id"`
}

// account resolves the signed-in account, or writes the refusal and reports
// false. Notifications belong to accounts, so an anonymous visitor has nothing
// to read here.
func account(c fiber.Ctx) (*user.Account, bool) {
	if !auth.Enabled() {
		_ = c.Status(fiber.StatusServiceUnavailable).
			JSON(errBody{Error: "accounts are unavailable in this environment"})
		return nil, false
	}
	acct := user.GetAccount(c)
	if acct == nil {
		_ = c.Status(fiber.StatusUnauthorized).
			JSON(errBody{Error: "log in to read notifications"})
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
	return c.JSON(listResponse{Unread: db.UnreadNotifications(acct.ID), Items: items})
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

// ReadAllHandler clears the account's unread backlog.
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
	return c.JSON(fiber.Map{"unread": db.UnreadNotifications(acct.ID)})
}
