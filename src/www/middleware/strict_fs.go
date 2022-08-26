package middleware

import (
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/dechristopher/lio/config"
	"github.com/dechristopher/lio/str"
	"github.com/dechristopher/lio/util"
)

// strictFs is a Custom strict filesystem implementation to
// prevent directory listings for resources
type strictFs struct {
	fs http.FileSystem
}

// Open only allows existing files to be pulled, not directories
func (sfs strictFs) Open(path string) (http.File, error) {
	// url decode path to support encoded characters
	path, err := url.QueryUnescape(path)
	if err != nil {
		util.Error(str.CFS, str.EFSDecode, path, err.Error())
		return nil, err
	}

	// strip cache key from static assets
	if strings.Contains(path, config.CacheKey) {
		path = strings.Replace(path, config.CacheKey, "", 1)
	}

	// trim trailing slashes to avoid invalid path errors
	// in fiber's filesystem middleware
	if path != "/" {
		path = strings.TrimSuffix(path, "/")
	}

	// fix accidental double slashes in filename
	path = strings.Replace(path, "//", "/", -1)

	// open file directly if it exists
	f, err := sfs.fs.Open(path)
	if err != nil {
		// return nothing if we hit an error so that we 404
		return nil, os.ErrNotExist
	}

	// prevent directory listings, only show index file if any
	s, err := f.Stat()
	if err == nil && s.IsDir() {
		index := strings.TrimSuffix(path, "/") + "/index.html"
		if _, err := sfs.fs.Open(index); err != nil {
			return nil, err
		}
	}
	return f, nil
}
