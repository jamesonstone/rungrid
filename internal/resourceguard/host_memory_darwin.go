//go:build darwin

package resourceguard

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func hostMemoryBytes() (uint64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	content, err := exec.CommandContext(ctx, "sysctl", "-n", "hw.memsize").Output()
	if err != nil {
		return 0, fmt.Errorf("read host memory: %w", err)
	}
	value, err := strconv.ParseUint(strings.TrimSpace(string(content)), 10, 64)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("parse host memory")
	}
	return value, nil
}
