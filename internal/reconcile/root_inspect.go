package reconcile

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

func inspectRoot(ctx context.Context, repository maintenance.Repository, runner maintenance.Runner) (rootState, error) {
	state := rootState{path: repository.Primary}
	branch, err := gitText(ctx, runner, state.path, "symbolic-ref", "--quiet", "--short", "HEAD")
	if err == nil {
		state.branch = branch
	}
	state.headOID, err = gitText(ctx, runner, state.path, "rev-parse", "HEAD")
	if err != nil {
		return state, fmt.Errorf("inspect primary HEAD: %w", err)
	}
	staged, err := gitPaths(ctx, runner, state.path, "diff", "--no-renames", "--cached", "--name-only", "-z")
	if err != nil {
		return state, fmt.Errorf("inspect staged paths: %w", err)
	}
	unstaged, err := gitPaths(ctx, runner, state.path, "diff", "--no-renames", "--name-only", "-z")
	if err != nil {
		return state, fmt.Errorf("inspect tracked paths: %w", err)
	}
	untracked, err := gitPaths(ctx, runner, state.path, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return state, fmt.Errorf("inspect untracked paths: %w", err)
	}
	unmerged, err := gitPaths(ctx, runner, state.path, "diff", "--name-only", "--diff-filter=U", "-z")
	if err != nil {
		return state, fmt.Errorf("inspect unmerged paths: %w", err)
	}
	state.ignored, err = gitPaths(ctx, runner, state.path, "ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return state, fmt.Errorf("inspect ignored paths: %w", err)
	}
	state.staged = len(staged) != 0
	state.conflicted = len(unmerged) != 0
	state.dirtyPaths = uniquePaths(staged, unstaged, untracked)
	state.commitAt = gitActivityTime(ctx, runner, state.path, "show", "-s", "--format=%ct", "HEAD")
	state.reflogAt = gitActivityTime(ctx, runner, state.path, "reflog", "-1", "--format=%ct", "HEAD")
	state.activityAt = newestTime(state.commitAt, state.reflogAt)
	for _, path := range state.dirtyPaths {
		candidate := filepath.Join(state.path, filepath.FromSlash(path))
		info, statErr := os.Lstat(candidate)
		if os.IsNotExist(statErr) {
			info, statErr = os.Stat(filepath.Dir(candidate))
		}
		if statErr == nil && info.ModTime().After(state.dirtyAt) {
			state.dirtyAt = info.ModTime()
		}
	}
	state.activityAt = newestTime(state.activityAt, state.dirtyAt)
	cwdPIDs, cwdErr := worktreeProcessIDs(ctx, runner, state.path, state.path)
	openPIDs, pathErr := dirtyPathProcessIDs(ctx, runner, state.path, state.path, state.dirtyPaths)
	state.cwdPIDs, state.openPIDs = cwdPIDs, openPIDs
	state.processes = uniqueStrings(state.cwdPIDs, state.openPIDs)
	if cwdErr != nil || pathErr != nil {
		state.processErr = fmt.Errorf("process activity inspection failed: %w", joinErrors(cwdErr, pathErr))
	}
	return state, nil
}

func gitActivityTime(ctx context.Context, runner maintenance.Runner, directory string, arguments ...string) time.Time {
	value, err := gitText(ctx, runner, directory, arguments...)
	if err != nil {
		return time.Time{}
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}

func newestTime(first, second time.Time) time.Time {
	if second.After(first) {
		return second
	}
	return first
}

func gitPaths(ctx context.Context, runner maintenance.Runner, directory string, arguments ...string) ([]string, error) {
	content, err := runner.Run(ctx, directory, "git", arguments...)
	if err != nil {
		return nil, err
	}
	parts := strings.Split(string(content), "\x00")
	result := parts[:0]
	for _, part := range parts {
		if part != "" {
			result = append(result, filepath.ToSlash(part))
		}
	}
	return result, nil
}

func uniquePaths(groups ...[]string) []string {
	seen := make(map[string]bool)
	for _, group := range groups {
		for _, path := range group {
			seen[path] = true
		}
	}
	result := make([]string, 0, len(seen))
	for path := range seen {
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func uniqueStrings(groups ...[]string) []string { return uniquePaths(groups...) }

func gitText(ctx context.Context, runner maintenance.Runner, directory string, arguments ...string) (string, error) {
	content, err := runner.Run(ctx, directory, "git", arguments...)
	return strings.TrimSpace(string(content)), err
}

func joinErrors(first, second error) error {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return fmt.Errorf("%v; %w", first, second)
}
