//go:build windows

package security

import (
	"fmt"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// AvailableBytes returns free bytes available to the calling user on targetDir's volume.
func AvailableBytes(targetDir string) (uint64, error) {
	abs, err := filepath.Abs(targetDir)
	if err != nil {
		return 0, err
	}
	ptr, err := windows.UTF16PtrFromString(abs)
	if err != nil {
		return 0, err
	}
	var freeAvail, totalBytes, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &freeAvail, &totalBytes, &totalFree); err != nil {
		return 0, fmt.Errorf("disk free space for %s: %w", abs, err)
	}
	return freeAvail, nil
}
