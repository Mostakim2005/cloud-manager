//go:build !windows

package security

import (
	"path/filepath"

	"golang.org/x/sys/unix"
)

// AvailableBytes returns free bytes available on targetDir's filesystem.
func AvailableBytes(targetDir string) (uint64, error) {
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		return 0, err
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(abs, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
