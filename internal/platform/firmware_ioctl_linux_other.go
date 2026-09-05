//go:build linux && !arm64

package platform

import (
	"io/fs"
	"os"
)

func firmwarePropertyIOCTL(*os.File, []byte) error { return fs.ErrNotExist }
