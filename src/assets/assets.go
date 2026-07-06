// Package assets provides content-hashed URLs for the embedded static assets so
// the browser cache busts exactly when a file's bytes change — and no more.
//
// The manifest is derived from the served static FS at startup (see www.Serve),
// so every instance of the same binary computes identical hashes: asset URLs are
// stable across instances and deploys. This replaces the old per-process random
// key (config.CacheKey), which changed on every restart (busting the cache
// needlessly) and broke multi-instance serving — an asset request for one
// instance's URL that landed on another instance failed to resolve and 404'd.
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"path"
	"sync"
)

var (
	mu      sync.RWMutex
	urls    = map[string]string{} // real path -> content-hashed public URL ("/lio-game.<hash>.js")
	reverse = map[string]string{} // hashed request path ("lio-game.<hash>.js") -> real path
)

// Build content-hashes every file in fsys and (re)populates the manifest. It is
// called once at startup, before the server accepts requests, so the read paths
// (URL/Real) need no coordination with it in practice; the RWMutex only guards
// against a pathological concurrent rebuild.
func Build(fsys fs.FS) error {
	nextURLs := map[string]string{}
	nextReverse := map[string]string{}

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		data, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		hashed := insertHash(p, hex.EncodeToString(sum[:])[:12])
		nextURLs[p] = "/" + hashed
		nextReverse[hashed] = p
		return nil
	})
	if err != nil {
		return err
	}

	mu.Lock()
	urls, reverse = nextURLs, nextReverse
	mu.Unlock()
	return nil
}

// URL returns the content-hashed public URL for a static asset named by its
// slash-relative real path (e.g. "lio-game.js" -> "/lio-game.<hash>.js"). If the
// manifest has no entry — not built yet, or an unknown file — it falls back to
// "/"+name so rendering never fails (the asset still serves, just uncached).
func URL(name string) string {
	mu.RLock()
	u, ok := urls[name]
	mu.RUnlock()
	if ok {
		return u
	}
	return "/" + name
}

// Real maps a hashed request path back to its real file (e.g.
// "lio-game.<hash>.js" -> "lio-game.js"). ok is false for paths the manifest
// does not know, which the static middleware serves unchanged.
func Real(name string) (string, bool) {
	mu.RLock()
	r, ok := reverse[name]
	mu.RUnlock()
	return r, ok
}

// insertHash puts ".<hash>" before a path's final extension:
//
//	"lio-game.js"            -> "lio-game.<hash>.js"
//	"octadground.base.css"   -> "octadground.base.<hash>.css"
//	"res/themes/board/g.css" -> "res/themes/board/g.<hash>.css"
func insertHash(p, h string) string {
	ext := path.Ext(p)
	return p[:len(p)-len(ext)] + "." + h + ext
}
