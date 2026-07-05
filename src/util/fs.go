package util

import (
	"embed"
	"io/fs"
	"os"
	"strings"

	"github.com/dechristopher/lio/str"
)

// PickFS returns either an embedded FS or an on-disk FS for the
// given directory path
func PickFS(useDisk bool, e embed.FS, dir string) fs.FS {
	if useDisk {
		Debug(str.CFS, str.DPickFSOS, dir)
		return os.DirFS(dir)
	}

	efs, err := fs.Sub(e, strings.Trim(dir, "./"))
	if err != nil {
		panic(err)
	}

	Debug(str.CFS, str.DPickFSEm, dir)
	return efs
}
