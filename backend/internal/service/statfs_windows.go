//go:build windows

package service

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func statfsBytes(path string) (total uint64, free uint64, avail uint64, err error) {
	var freeBytesAvailable, totalNumberOfBytes, totalNumberOfFreeBytes uint64

	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("failed to convert path: %w", err)
	}

	err = windows.GetDiskFreeSpaceEx(
		pathPtr,
		&freeBytesAvailable,
		&totalNumberOfBytes,
		&totalNumberOfFreeBytes,
	)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("GetDiskFreeSpaceEx failed: %w", err)
	}

	total = totalNumberOfBytes
	free = totalNumberOfFreeBytes
	avail = freeBytesAvailable

	return total, free, avail, nil
}
