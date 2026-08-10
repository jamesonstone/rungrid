package reconcile

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

func worktreeProcessIDs(ctx context.Context, runner maintenance.Runner, directory, root string) ([]string, error) {
	content, err := runner.Run(ctx, directory, "lsof", "-a", "-d", "cwd", "-Fn")
	if err != nil {
		return nil, fmt.Errorf("inspect cwd processes: %w", err)
	}
	return parseLsofPaths(content, root), nil
}

func dirtyPathProcessIDs(ctx context.Context, runner maintenance.Runner, directory, root string, paths []string) ([]string, error) {
	arguments := []string{"-Fn", "--"}
	for _, path := range paths {
		absolute := filepath.Join(root, filepath.FromSlash(path))
		if _, err := os.Lstat(absolute); err == nil {
			arguments = append(arguments, absolute)
		}
	}
	if len(arguments) == 2 {
		return nil, nil
	}
	content, err := runner.Run(ctx, directory, "lsof", arguments...)
	if err != nil {
		var exitError *exec.ExitError
		if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
			return nil, nil
		}
		return nil, fmt.Errorf("inspect dirty path processes: %w", err)
	}
	return parseLsofPaths(content, root), nil
}

func parseLsofPaths(content []byte, root string) []string {
	currentPID := ""
	seen := make(map[string]bool)
	for _, line := range strings.Split(string(content), "\n") {
		switch {
		case strings.HasPrefix(line, "p"):
			currentPID = strings.TrimPrefix(line, "p")
		case strings.HasPrefix(line, "n") && currentPID != "":
			if pathInside(root, strings.TrimPrefix(line, "n")) && currentPID != fmt.Sprint(os.Getpid()) {
				seen[currentPID] = true
			}
		}
	}
	result := make([]string, 0, len(seen))
	for pid := range seen {
		result = append(result, pid)
	}
	sort.Strings(result)
	return result
}

func pathInside(root, candidate string) bool {
	root = physicalPath(root)
	candidate = physicalPath(candidate)
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(os.PathSeparator))
}

func physicalPath(value string) string {
	if resolved, err := filepath.EvalSymlinks(value); err == nil {
		return resolved
	}
	return filepath.Clean(value)
}

type processGuardCoordinator struct {
	delegate maintenance.Coordinator
	runner   maintenance.Runner
}

func (guard processGuardCoordinator) AffectedServices(ctx context.Context, path string) ([]string, error) {
	return guard.delegate.AffectedServices(ctx, path)
}

func (guard processGuardCoordinator) OwnedProcessIDs(ctx context.Context, path string) ([]string, error) {
	owner, ok := guard.delegate.(maintenance.ProcessOwner)
	if !ok {
		return nil, nil
	}
	return owner.OwnedProcessIDs(ctx, path)
}

func (guard processGuardCoordinator) Pause(ctx context.Context, path string) ([]string, maintenance.ResumeFunc, error) {
	var owned []string
	if owner, ok := guard.delegate.(maintenance.ProcessOwner); ok && path != "" {
		var ownerErr error
		owned, ownerErr = owner.OwnedProcessIDs(ctx, path)
		if ownerErr != nil {
			return nil, nil, fmt.Errorf("inspect coordinator-owned processes: %w", ownerErr)
		}
	}
	if path != "" {
		processes, inspectErr := worktreeProcessIDs(ctx, guard.runner, path, path)
		if inspectErr != nil {
			return nil, nil, inspectErr
		}
		if unowned := withoutStrings(processes, owned); len(unowned) != 0 {
			return nil, nil, fmt.Errorf("unowned cwd processes use default worktree: %s", strings.Join(unowned, ","))
		}
	}
	services, resume, err := guard.delegate.Pause(ctx, path)
	if err != nil || path == "" {
		return services, resume, err
	}
	processes, inspectErr := worktreeProcessIDs(ctx, guard.runner, path, path)
	processes = withoutStrings(processes, owned)
	if inspectErr == nil && len(processes) == 0 {
		return services, resume, nil
	}
	recoveryContext, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	resumeErr := resume(recoveryContext)
	cancel()
	if inspectErr != nil {
		return services, func(context.Context) error { return nil }, errors.Join(inspectErr, resumeErr)
	}
	return services, func(context.Context) error { return nil }, errors.Join(
		fmt.Errorf("unowned cwd processes remain in default worktree: %s", strings.Join(processes, ",")), resumeErr)
}

func withoutStrings(values, excluded []string) []string {
	blocked := make(map[string]bool, len(excluded))
	for _, value := range excluded {
		blocked[value] = true
	}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !blocked[value] {
			result = append(result, value)
		}
	}
	return result
}
