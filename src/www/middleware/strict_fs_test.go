package middleware

import (
	"io"
	"testing"
	"testing/fstest"

	"github.com/dechristopher/lio/assets"
)

// TestStrictFsResolvesHashedURL proves the serving path: a content-hashed public
// URL (as emitted into page HTML by view.asset) resolves back through the assets
// manifest to the real file, and a stale/bogus hash does not.
func TestStrictFsResolvesHashedURL(t *testing.T) {
	fsys := fstest.MapFS{
		"lio.js": {Data: []byte("console.log(1)")},
	}
	if err := assets.Build(fsys); err != nil {
		t.Fatal(err)
	}
	sfs := strictFs{fsys}

	// the hashed URL (minus its leading slash) must open the real file — this is
	// exactly what a browser requests and what the static middleware passes in.
	hashed := assets.URL("lio.js")[1:] // "lio.<hash>.js"
	f, err := sfs.Open(hashed)
	if err != nil {
		t.Fatalf("Open(%q): %v", hashed, err)
	}
	if b, _ := io.ReadAll(f); string(b) != "console.log(1)" {
		t.Fatalf("wrong content: %q", b)
	}

	// a bogus/stale hash is not in the manifest, so it passes through unchanged
	// and misses (no accidental fallback to the real file).
	if _, err := sfs.Open("lio.deadbeef00.js"); err == nil {
		t.Fatalf("stale hash should not resolve to the real file")
	}
}
