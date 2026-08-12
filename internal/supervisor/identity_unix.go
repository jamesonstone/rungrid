//go:build darwin || linux

package supervisor

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/subprocess"
)

func waitForSocket(ctx context.Context, socket string) error {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if info, err := os.Lstat(socket); err == nil && info.Mode()&os.ModeSocket != 0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(errs.ExitNotReady, "RG625", "wait for Process Compose Unix socket", ctx.Err())
		case <-ticker.C:
		}
	}
}

func socketPID(ctx context.Context, socket, configuration string) (int, error) {
	result, err := subprocess.Run(exec.CommandContext(ctx, "lsof", "-nP", "-U", "-F", "pcn"))
	if err != nil {
		return 0, errs.Wrap(errs.ExitDependency, "RG626", "identify Process Compose socket owner with lsof", err)
	}
	pid := 0
	commandName := ""
	for _, line := range strings.Split(string(result.Stdout), "\n") {
		if len(line) < 2 {
			continue
		}
		switch line[0] {
		case 'p':
			pid, _ = strconv.Atoi(line[1:])
			commandName = ""
		case 'c':
			commandName = line[1:]
		case 'n':
			if pid <= 1 || !strings.Contains(strings.ToLower(commandName), "process-c") || !reportedSocketMatches(line[1:], socket, filepath.Dir(configuration)) {
				continue
			}
			_, processCommand, inspectErr := inspectProcess(ctx, pid)
			if inspectErr == nil && strings.Contains(processCommand, configuration) {
				return pid, nil
			}
		}
	}
	return 0, errs.New(errs.ExitConflict, "RG627", "no process owns the Process Compose socket")
}

func reportedSocketMatches(reported, expected, processDirectory string) bool {
	if suffix := strings.Index(reported, " type="); suffix >= 0 {
		reported = reported[:suffix]
	}
	reported = strings.TrimSpace(reported)
	if reported == "" {
		return false
	}
	if filepath.IsAbs(reported) {
		return filepath.Clean(reported) == filepath.Clean(expected)
	}
	return filepath.Clean(filepath.Join(processDirectory, reported)) == filepath.Clean(expected)
}

func inspectProcess(ctx context.Context, pid int) (string, string, error) {
	if pid <= 1 || !processExists(pid) {
		return "", "", errs.New(errs.ExitConflict, "RG628", "runtime process does not exist")
	}
	identityCommand := exec.CommandContext(ctx, "ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	identityCommand.Env = cLocaleEnvironment(os.Environ())
	identityResult, err := subprocess.Run(identityCommand)
	if err != nil {
		return "", "", errs.Wrap(errs.ExitConflict, "RG629", "inspect runtime process start identity", err)
	}
	commandResult, err := subprocess.Run(exec.CommandContext(ctx, "ps", "-o", "command=", "-p", strconv.Itoa(pid)))
	if err != nil {
		return "", "", errs.Wrap(errs.ExitConflict, "RG630", "inspect runtime process command", err)
	}
	identity := strings.Join(strings.Fields(string(identityResult.Stdout)), " ")
	command := strings.TrimSpace(string(commandResult.Stdout))
	if identity == "" || command == "" || !strings.Contains(strings.ToLower(command), "process-compose") {
		return "", "", errs.New(errs.ExitConflict, "RG631", "runtime PID is not Process Compose")
	}
	return identity, command, nil
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

func inspectSocket(filename string) (uint64, uint64, uint32, error) {
	info, err := os.Lstat(filename)
	if err != nil {
		return 0, 0, 0, errs.Wrap(errs.ExitConflict, "RG632", "inspect runtime Unix socket", err)
	}
	if info.Mode()&os.ModeSocket == 0 || info.Mode()&os.ModeSymlink != 0 {
		return 0, 0, 0, errs.New(errs.ExitConflict, "RG633", "runtime path is not a Unix socket")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0, 0, 0, errs.New(errs.ExitConflict, "RG634", "Unix socket identity is unavailable")
	}
	return uint64(stat.Dev), uint64(stat.Ino), stat.Uid, nil
}

func processExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}
