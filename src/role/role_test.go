package role

import "testing"

// TestParseFailsClosed locks the rule that an unrecognized (or empty) stored
// role reads as Player. A role string that somehow escaped the users.role CHECK
// constraint must not be treated as privileged.
func TestParseFailsClosed(t *testing.T) {
	for _, s := range []string{"", "PLAYER", "moderator", "root", "Admin", "  admin"} {
		if got := Parse(s); got != Player {
			t.Errorf("Parse(%q) = %q, want %q", s, got, Player)
		}
	}
	if got := Parse("mod"); got != Mod {
		t.Errorf("Parse(\"mod\") = %q, want %q", got, Mod)
	}
	if got := Parse("admin"); got != Admin {
		t.Errorf("Parse(\"admin\") = %q, want %q", got, Admin)
	}
}

// TestLadder covers the ordering predicates the whole authorization surface
// reads through: who may moderate at all, and who may administer.
func TestLadder(t *testing.T) {
	cases := []struct {
		r                      Role
		moderate, admin        bool
		atLeastMod, atLeastAdm bool
	}{
		{Role(""), false, false, false, false},
		{Player, false, false, false, false},
		{Mod, true, false, true, false},
		{Admin, true, true, true, true},
	}
	for _, tc := range cases {
		if got := tc.r.CanModerate(); got != tc.moderate {
			t.Errorf("%q.CanModerate() = %v, want %v", tc.r, got, tc.moderate)
		}
		if got := tc.r.CanAdmin(); got != tc.admin {
			t.Errorf("%q.CanAdmin() = %v, want %v", tc.r, got, tc.admin)
		}
		if got := tc.r.AtLeast(Mod); got != tc.atLeastMod {
			t.Errorf("%q.AtLeast(Mod) = %v, want %v", tc.r, got, tc.atLeastMod)
		}
		if got := tc.r.AtLeast(Admin); got != tc.atLeastAdm {
			t.Errorf("%q.AtLeast(Admin) = %v, want %v", tc.r, got, tc.atLeastAdm)
		}
	}
}

// TestCanActOn is the rule that keeps moderators from policing each other: a
// mod may only act on ordinary players, while an admin may act on anyone. (The
// no-self-action and last-admin rules live in the handlers — this predicate
// answers only the ladder question.)
func TestCanActOn(t *testing.T) {
	cases := []struct {
		actor, target Role
		want          bool
	}{
		{Player, Player, false}, // not a moderator at all
		{Player, Mod, false},
		{Mod, Player, true},
		{Mod, Role(""), true}, // zero target is an ordinary player
		{Mod, Mod, false},     // no mod-on-mod
		{Mod, Admin, false},
		{Admin, Player, true},
		{Admin, Mod, true},
		{Admin, Admin, true},
	}
	for _, tc := range cases {
		if got := tc.actor.CanActOn(tc.target); got != tc.want {
			t.Errorf("%q.CanActOn(%q) = %v, want %v", tc.actor, tc.target, got, tc.want)
		}
	}
}
