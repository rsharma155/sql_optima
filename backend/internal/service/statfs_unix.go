//go:build darwin || linux

package service

import (
	"fmt"
	"syscall"
)

func statfsBytes(path string) (total uint64, free uint64, avail uint64, err error) {
	var st syscall.Statfs_t
	if e := syscall.Statfs(path, &st); e != nil {
		return 0, 0, 0, fmt.Errorf("statfs failed: %w", e)
	}
	// Use uint64 to avoid overflow when multiplying.
	bsize := uint64(st.Bsize)
	total = st.Blocks * bsize
	free = st.Bfree * bsize
	avail = st.Bavail * bsize
	return total, free, avail, nil
}

