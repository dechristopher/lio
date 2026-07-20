package view

// SessionView is one row of the profile popover's active-sessions list
// (arch/ACCOUNTS_AUTH_RATINGS.md Phase 3). The handler formats Device (coarse
// "Browser on OS") and LastSeen (relative time) so the template stays dumb;
// Current marks the requesting session, which is labeled and not revocable
// from the list.
type SessionView struct {
	ID       int64
	Device   string
	LastSeen string
	Current  bool
}
