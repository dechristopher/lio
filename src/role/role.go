// Package role models an account's site permission level: the ladder
// player < mod < admin behind site administration and moderation
// (arch/ADMIN_MODERATION.md).
//
// Unlike titles (see the title package), roles are code-coupled — the server
// has to know what a "mod" may do, so the set lives here and in the users.role
// CHECK constraint, not in a table. This is the display/authorization value
// carried through the session and the render; the column is the source of
// truth.
//
// The zero Role is the empty string, which Parse and every predicate treat as
// Player — an anonymous visitor and an ordinary account are equally unprivileged,
// so a zero-valued Role can be threaded and checked unconditionally.
package role

// Role is an account's permission level.
type Role string

const (
	// Player is every ordinary account (and the zero value's meaning).
	Player Role = "player"
	// Mod may sanction players: ban/unban, forced rename, resolve reports.
	// Deliberately cannot act on another Mod or an Admin.
	Mod Role = "mod"
	// Admin may do everything a Mod may, plus set roles (appointing further
	// mods) and change site controls.
	Admin Role = "admin"
)

// rank orders the ladder. Anything unrecognized ranks at Player, so a role
// string that somehow escaped the CHECK constraint fails closed (unprivileged)
// rather than open.
func (r Role) rank() int {
	switch r {
	case Admin:
		return 2
	case Mod:
		return 1
	default:
		return 0
	}
}

// Parse normalizes a stored role string, mapping "" and anything unrecognized
// to Player. Used at the db boundary so nothing above it has to handle a
// surprise value.
func Parse(s string) Role {
	switch Role(s) {
	case Admin:
		return Admin
	case Mod:
		return Mod
	default:
		return Player
	}
}

// String returns the canonical stored form; the zero Role reads as Player.
func (r Role) String() string {
	return string(Parse(string(r)))
}

// AtLeast reports whether r sits at or above other on the ladder.
func (r Role) AtLeast(other Role) bool {
	return r.rank() >= other.rank()
}

// CanModerate reports whether the role may take moderation actions against
// players at all — the gate on the whole /api/mod surface and on rendering the
// mod bar.
func (r Role) CanModerate() bool {
	return r.AtLeast(Mod)
}

// CanAdmin reports whether the role may set other accounts' roles and change
// site controls.
func (r Role) CanAdmin() bool {
	return r.AtLeast(Admin)
}

// CanActOn reports whether an actor with role r may take a moderation action
// against a target holding role target. Mods may only act on ordinary players:
// a mod cannot ban, retitle or rename a peer or an admin. Admins may act on
// anyone (the "don't demote the last admin" and "no self-action" rules are
// separate checks the handlers make — this predicate only answers the ladder
// question).
func (r Role) CanActOn(target Role) bool {
	if !r.CanModerate() {
		return false
	}
	if r.CanAdmin() {
		return true
	}
	return !Parse(string(target)).CanModerate()
}
