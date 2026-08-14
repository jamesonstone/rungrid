//go:build darwin || linux

package procidentity

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/subprocess"
)

func Current() (string, error) { return Inspect(os.Getpid()) }

func Inspect(pid int) (string, error) {
	if pid <= 1 {
		return "", fmt.Errorf("invalid PID")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, "ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	command.Env = cLocaleEnvironment(os.Environ())
	result, err := subprocess.Run(command)
	if err != nil {
		return "", err
	}
	identity := strings.Join(strings.Fields(string(result.Stdout)), " ")
	if identity == "" {
		return "", fmt.Errorf("empty process start identity")
	}
	return identity, nil
}

func cLocaleEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, value := range environment {
		if !strings.HasPrefix(value, "LC_ALL=") && !strings.HasPrefix(value, "LANG=") {
			result = append(result, value)
		}
	}
	return append(result, "LC_ALL=C")
}

func Matches(pid int, identity string) bool {
	if identity == "" {
		return false
	}
	process, err := os.FindProcess(pid)
	if err != nil || process.Signal(syscall.Signal(0)) != nil {
		return false
	}
	actual, err := Inspect(pid)
	return err == nil && actual == identity
}
