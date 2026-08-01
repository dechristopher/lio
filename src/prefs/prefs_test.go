package prefs

import "testing"

// TestZeroSnapshotIsDefaults locks the property the whole package leans on: a
// Snapshot nobody filled in reads as "this player has never changed anything".
// It is what an anonymous viewer gets, what a failed database read falls back
// to, and what a component test renders against — all three must agree with a
// signed-in player who holds no rows.
func TestZeroSnapshotIsDefaults(t *testing.T) {
	var zero Snapshot
	if !zero.ShowHomeAbout() {
		t.Error("zero Snapshot hides the home explainer; it is on by default")
	}
	for key, want := range flags {
		if got := zero.Flag(key); got != want {
			t.Errorf("zero Snapshot %s = %v, want the default %v", key, got, want)
		}
	}
}

// TestFlagReadsOverrides covers a stored value winning over the default, in
// both directions.
func TestFlagReadsOverrides(t *testing.T) {
	off := Snapshot{raw: map[string]string{KeyHomeAbout: "0"}}
	if off.ShowHomeAbout() {
		t.Error("explainer shown with the preference stored off")
	}
	on := Snapshot{raw: map[string]string{KeyHomeAbout: "1"}}
	if !on.ShowHomeAbout() {
		t.Error("explainer hidden with the preference stored on")
	}
	// anything that is not "1" reads as off, so a value written by some future
	// version can never resolve to a third state
	junk := Snapshot{raw: map[string]string{KeyHomeAbout: "yes"}}
	if junk.ShowHomeAbout() {
		t.Error("unrecognized value did not read as off")
	}
}

// TestWithCopies checks the test-arrangement helper sets the flag it is given
// and leaves the receiver alone — a Snapshot handed out of the cache is shared.
func TestWithCopies(t *testing.T) {
	base := Snapshot{raw: map[string]string{KeyHomeAbout: "1"}}
	hidden := base.With(KeyHomeAbout, false)
	if hidden.ShowHomeAbout() {
		t.Error("With(false) did not turn the preference off")
	}
	if !base.ShowHomeAbout() {
		t.Error("With mutated the receiver")
	}
	// and it works from the zero value, which is the case a test starts from
	if (Snapshot{}).With(KeyHomeAbout, false).Flag(KeyHomeAbout) {
		t.Error("With from the zero Snapshot did not take")
	}
}

// TestValidRejectsUnknownKeys guards the endpoint's whitelist: a client must
// not be able to write arbitrary rows into an account's preference set.
func TestValidRejectsUnknownKeys(t *testing.T) {
	if !Valid(KeyHomeAbout) {
		t.Errorf("%s rejected as a key", KeyHomeAbout)
	}
	for _, key := range []string{"", "home", "home.about.x", "role", "1"} {
		if Valid(key) {
			t.Errorf("unknown key %q accepted", key)
		}
	}
}

// TestForWithoutPostgres covers the PG-less local dev path: no pool means no
// stored preferences, which must resolve to the defaults rather than failing.
func TestForWithoutPostgres(t *testing.T) {
	Invalidate(1)
	if !For(1).ShowHomeAbout() {
		t.Error("preferences did not default open without a database")
	}
	// a zero id is "no account", answered without touching storage at all
	if !For(0).ShowHomeAbout() {
		t.Error("anonymous viewer did not read the defaults")
	}
}
