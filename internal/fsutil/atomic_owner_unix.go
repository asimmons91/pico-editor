//go:build unix

package fsutil

import (
	"fmt"
	"os"
	"syscall"
)

// preserveOwner keeps the original uid/gid across the atomic rename. If the
// owner cannot be preserved, the save fails rather than silently changing it.
func preserveOwner(path string, info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	if err := os.Chown(path, int(stat.Uid), int(stat.Gid)); err != nil {
		return fmt.Errorf("preserve owner of %s: %w", path, err)
	}
	return nil
}
