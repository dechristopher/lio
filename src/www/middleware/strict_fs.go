package middleware

import (
	"io/fs"
	"strings"

	"github.com/dechristopher/lio/config"
)

// strictFs wraps a base fs.FS and strips the static-asset cache-busting key
// (config.CacheKey, e.g. ".<hash>") from requested paths, so cache-busted URLs
// like "app.<key>.css" resolve to the real "app.css" on disk. Directory-listing
// prevention and the index.html fallback that the old http.FileSystem
// implementation handled by hand are now provided by the static middleware
// (Browse defaults to false, IndexNames defaults to index.html).
type strictFs struct {
	fs fs.FS
}

// Open strips the cache key from the requested name and delegates to the base
// FS. The static middleware passes cleaned, url-decoded, slash-relative paths,
// so no further path normalization is needed here.
func (sfs strictFs) Open(name string) (fs.File, error) {
	// strip cache key from static assets (e.g. app.<key>.css -> app.css)
	if strings.Contains(name, config.CacheKey) {
		name = strings.Replace(name, config.CacheKey, "", 1)
	}

	return sfs.fs.Open(name)
}
