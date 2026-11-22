//go:build unix

package checkpoint

import (
	"os"
	"syscall"
)

func allocated(info os.FileInfo) uint64 {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		return uint64(stat.Blocks) * 512
	}
	return uint64(info.Size())
}
