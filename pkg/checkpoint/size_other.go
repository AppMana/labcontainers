//go:build !unix

package checkpoint

import "os"

func allocated(info os.FileInfo) uint64 { return uint64(info.Size()) }
