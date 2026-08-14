//go:build linux

package resourceguard

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func hostMemoryBytes() (uint64, error) {
	var info unix.Sysinfo_t
	if err := unix.Sysinfo(&info); err != nil {
		return 0, fmt.Errorf("read host memory: %w", err)
	}
	value := info.Totalram * uint64(info.Unit)
	if value == 0 {
		return 0, fmt.Errorf("host memory is unavailable")
	}
	return value, nil
}
