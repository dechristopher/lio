package middleware

import (
	"io/fs"

	"github.com/dechristopher/lio/assets"
)

// strictFs wraps a base fs.FS and resolves content-hashed static-asset URLs
// (e.g. "app.<hash>.css") back to the real file on disk ("app.css") via the
// assets manifest. Directory-listing prevention and the index.html fallback that
// the old http.FileSystem implementation handled by hand are now provided by the
// static middleware (Browse defaults to false, IndexNames defaults to index.html).
type strictFs struct {
	fs fs.FS
}

// Open maps a content-hashed request path back to its real file and delegates to
// the base FS. The static middleware passes cleaned, url-decoded, slash-relative
// paths, so no further path normalization is needed here. Unknown (unhashed)
// paths are served unchanged.
func (sfs strictFs) Open(name string) (fs.File, error) {
	if real, ok := assets.Real(name); ok {
		name = real
	}

	return sfs.fs.Open(name)
}
