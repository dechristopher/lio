package util

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"

	"github.com/dechristopher/lioctad/str"
)

// PickFS returns either an embedded FS or an on-disk FS for the
// given directory path
func PickFS(useDisk bool, e embed.FS, dir string) http.FileSystem {
	if useDisk {
		Debug(str.CPickFS, str.DPickFSOS, dir)
		return http.Dir(dir)
	}

	efs, err := fs.Sub(e, strings.Trim(dir, "./"))
	if err != nil {
		panic(err)
	}

	Debug(str.CPickFS, str.DPickFSEm, dir)
	return http.FS(efs)
}
