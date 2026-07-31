package mod

import (
	"testing"

	"github.com/dechristopher/lio/role"
)

// What an account is told when its role changes (arch/NOTIFICATIONS.md). The
// copy is the whole feature here: an appointment is invisible until the person
// happens to look for the tools, so a wrong or missing sentence is the same as
// no notification at all.
func TestRoleNotice(t *testing.T) {
	cases := []struct {
		name     string
		from, to role.Role
		body     string
		link     string
	}{
		{"appoint a moderator", role.Player, role.Mod,
			"You are now a moderator.", "/system"},
		{"appoint an admin", role.Player, role.Admin,
			"You are now an administrator.", "/system"},
		{"promote a moderator", role.Mod, role.Admin,
			"You are now an administrator.", "/system"},
		// Standing an admin down to moderator is an appointment as much as a
		// removal: they keep tools, and the sentence names the ones they keep.
		{"stand an admin down to moderator", role.Admin, role.Mod,
			"You are now a moderator.", "/system"},
		// No link on a removal. The page it would point at is the one the
		// account no longer has.
		{"remove moderator access", role.Mod, role.Player,
			"Your moderator access has been removed.", ""},
		{"remove admin access", role.Admin, role.Player,
			"Your administrator access has been removed.", ""},
		// Nothing changed, so there is nothing to announce. The handler refuses
		// a no-op role write above this, but a message about a change that did
		// not happen would be worse than a wasted request.
		{"no change", role.Mod, role.Mod, "", ""},
		{"player to player", role.Player, role.Player, "", ""},
		// The zero Role is Player everywhere else, and must be here too, or a
		// first appointment from an unset role would announce nothing.
		{"zero role appointed", role.Role(""), role.Mod,
			"You are now a moderator.", "/system"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, link := roleNotice(tc.from, tc.to)
			if body != tc.body {
				t.Errorf("body = %q, want %q", body, tc.body)
			}
			if link != tc.link {
				t.Errorf("link = %q, want %q", link, tc.link)
			}
		})
	}
}
