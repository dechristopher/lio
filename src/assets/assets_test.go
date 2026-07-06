package assets

import (
	"testing"
	"testing/fstest"
)

// TestBuildURLReal exercises the manifest round-trip: URL produces a content-
// hashed public path, Real maps it back, and the hash is inserted before the
// final extension (including multi-dot names like octadground.base.css).
func TestBuildURLReal(t *testing.T) {
	fsys := fstest.MapFS{
		"app.css":                    {Data: []byte("body{}")},
		"lio-game.js":                {Data: []byte("var x=1")},
		"octadground.base.css":       {Data: []byte(".og{}")},
		"res/themes/board/green.css": {Data: []byte(".b{}")},
	}
	if err := Build(fsys); err != nil {
		t.Fatalf("Build: %v", err)
	}

	for _, name := range []string{"app.css", "lio-game.js", "octadground.base.css", "res/themes/board/green.css"} {
		u := URL(name)
		if u == "/"+name {
			t.Fatalf("URL(%q) not hashed: %s", name, u)
		}
		// the public URL is "/<real-with-hash>"; Real takes the path without the
		// leading slash and must recover the original name.
		real, ok := Real(u[1:])
		if !ok || real != name {
			t.Fatalf("Real(%q) = %q,%v; want %q", u[1:], real, ok, name)
		}
	}

	// hash goes before the final extension, keeping intermediate dots.
	if got := URL("octadground.base.css"); got[:len("/octadground.base.")] != "/octadground.base." || got[len(got)-4:] != ".css" {
		t.Fatalf("octadground.base.css hashed wrong: %s", got)
	}

	// unknown / not-built files fall back to a plain path, never panic.
	if got := URL("nope.js"); got != "/nope.js" {
		t.Fatalf("URL(unknown) = %s; want /nope.js", got)
	}
	if _, ok := Real("nope.js"); ok {
		t.Fatalf("Real(unknown) should be false")
	}

	// identical bytes hash identically (stable across instances); different
	// bytes differ.
	if URL("app.css") == URL("lio-game.js") {
		t.Fatalf("distinct files must not share a hashed URL")
	}
}
