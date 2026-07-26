package user

import (
	"context"

	"github.com/gofiber/fiber/v3"

	"github.com/dechristopher/lio/role"
	"github.com/dechristopher/lio/title"
)

// Context is the request-scoped identity, resolved per request from the sid
// session cookie by auth.SessionMiddleware (arch/ACCOUNTS_AUTH_RATINGS.md).
// It replaced the old encrypted-cookie identity: nothing here is serialized
// to the client anymore — the cookie carries only an opaque session token.
type Context struct {
	context.Context
	// ID is the session's uid: the seat/socket identity rooms, sockets and
	// the games archive key off (16-char base58).
	ID string
	// Account is the attached account, nil for anonymous sessions.
	Account *Account
}

// Account identifies a logged-in user. Defined here (not in auth) so the auth
// package can depend on user without a cycle.
type Account struct {
	ID       int64
	Username string
	// Title is the account's optional display title (the zero Title when
	// unset), rendered to the left of the username in the theme accent color.
	// Assigned by pointing users.title_id at a titles row (no in-app
	// assignment UI); flows here from the session.
	Title title.Title
	// Role is the account's site permission level (arch/ADMIN_MODERATION.md),
	// Player for the vast majority. The render reads it to decide whether to
	// show the moderation UI; every privileged route re-checks it server-side,
	// so a hidden control is never the security boundary.
	Role role.Role
}

// GetID is a helper to return the session uid from the request context.
// Empty when no session resolved (cookie-less WS upgrades).
func GetID(ctx fiber.Ctx) string {
	c := GetContext(ctx)
	if c == nil {
		return ""
	}
	return c.ID
}

// GetContext returns the resolved identity Context from the fiber context.
func GetContext(ctx fiber.Ctx) *Context {
	c, ok := ctx.Context().(*Context)
	if ok {
		return c
	}

	return nil
}

// GetAccount returns the logged-in account for the request, or nil for
// anonymous visitors.
func GetAccount(ctx fiber.Ctx) *Account {
	c := GetContext(ctx)
	if c == nil {
		return nil
	}
	return c.Account
}
