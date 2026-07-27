package pools

import "testing"

// TestSpeedFor locks the deploy → speed resolution the player page uses. Deploy
// is the default mode, so a row labelled "deploy" tells a reader nothing; the
// speed class is the distinction they actually read.
func TestSpeedFor(t *testing.T) {
	cases := []struct{ name, group, want string }{
		{"¼ + 0", "deploy", "bullet"},
		{"½ + 1", "deploy", "blitz"},
		{"1 + 2", "deploy", "rapid"},
		{"3 + 5", "deploy", "rapid"},
		// A non-deploy variant's group already is its speed — but it resolves to
		// the same class as its deploy twin, so the mode has to disambiguate or
		// "½ + 1 deploy" and "½ + 1 blitz" both render as "½ + 1 blitz".
		{"½ + 1", "blitz", "blitz Classic"},
		{"1 + 2", "rapid", "rapid Classic"},
		{"∞", "unlimited", "unlimited"},
		// a retired pool still reads as something
		{"9 + 9", "nonesuch", "nonesuch"},
	}
	for _, c := range cases {
		if got := SpeedFor(c.name, c.group); got != c.want {
			t.Errorf("SpeedFor(%q, %q) = %q, want %q", c.name, c.group, got, c.want)
		}
	}
}
