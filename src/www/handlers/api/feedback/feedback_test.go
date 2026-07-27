package feedback

import "testing"

// TestSafePath pins the narrowing that stands between a client-supplied string
// and an href rendered on a privileged page. Everything that is not
// unambiguously a same-origin path on this site has to come back empty — the
// inbox links it, so a value that escaped here would be an attacker-chosen
// destination sitting on /system.
func TestSafePath(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain path", "/about", "/about"},
		{"root", "/", "/"},
		{"room permalink", "/Ab3xY9/2", "/Ab3xY9/2"},
		{"trimmed", "  /news  ", "/news"},
		{"query dropped", "/db?page=3", "/db"},
		{"fragment dropped", "/about#rules", "/about"},
		{"query and fragment", "/db?page=3#top", "/db"},

		{"empty", "", ""},
		{"blank", "   ", ""},
		{"relative", "about", ""},
		{"absolute http", "https://evil.example/x", ""},
		{"protocol relative", "//evil.example/x", ""},
		{"javascript scheme", "javascript:alert(1)", ""},
		{"data scheme", "data:text/html,<script>", ""},
		{"newline injection", "/about\n/evil", ""},
		{"tab injection", "/about\tx", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := safePath(c.in); got != c.want {
				t.Errorf("safePath(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestSafePathLengthBound covers the ceiling: an over-long value is dropped
// rather than truncated, because half a path is not a path.
func TestSafePathLengthBound(t *testing.T) {
	long := "/" + string(make([]byte, maxPath))
	if got := safePath(long); got != "" {
		t.Errorf("over-long path survived: %q", got)
	}
	atLimit := "/" + string(repeat('a', maxPath-1))
	if got := safePath(atLimit); got != atLimit {
		t.Errorf("path at the limit was dropped: %q", got)
	}
}

func repeat(b byte, n int) []byte {
	out := make([]byte, n)
	for i := range out {
		out[i] = b
	}
	return out
}
