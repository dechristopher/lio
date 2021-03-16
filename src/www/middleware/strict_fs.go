package middleware

import (
	"net/http"
	"strings"
)

// strictFs is a Custom strict filesystem implementation to
// prevent directory listings for resources
type strictFs struct {
	fs http.FileSystem
}

// Open only allows existing files to be pulled, not directories
func (sfs strictFs) Open(path string) (http.File, error) {
	// decode spaces back into path
	if strings.Contains(path, "%20") {
		path = strings.Replace(path, "%20", " ", -1)
	}

	// trim trailing slashes to avoid invalid path errors
	// in fiber's filesystem middleware
	path = strings.TrimSuffix(path, "/")

	f, err := sfs.fs.Open(path)
	if err != nil {
		return nil, err
	}

	s, err := f.Stat()
	if err == nil && s.IsDir() {
		index := strings.TrimSuffix(path, "/") + "/index.html"
		if _, err := sfs.fs.Open(index); err != nil {
			return nil, err
		}
	}
	return f, nil
}
