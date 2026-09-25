package checkout

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

func listWorktrees(ctx context.Context, runner maintenance.Runner, directory string) ([]Worktree, error) {
	content, err := git(ctx, runner, directory, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var result []Worktree
	var current *Worktree
	for _, line := range strings.Split(string(content), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path, pathErr := physicalPath(strings.TrimPrefix(line, "worktree "))
			if pathErr != nil {
				return nil, fmt.Errorf("resolve registered worktree: %w", pathErr)
			}
			result = append(result, Worktree{Path: path})
			current = &result[len(result)-1]
		case current == nil:
			continue
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			current.Branch = strings.TrimPrefix(line, "branch refs/heads/")
		case line == "detached":
			current.Detached = true
		case strings.HasPrefix(line, "locked"):
			current.Locked = true
		}
	}
	if len(result) > 0 {
		result[0].Primary = true
	}
	return result, nil
}

func git(ctx context.Context, runner maintenance.Runner, directory string, arguments ...string) ([]byte, error) {
	if runner == nil {
		runner = maintenance.CommandRunner{}
	}
	return runner.Run(ctx, directory, "git", arguments...)
}

func gitText(ctx context.Context, runner maintenance.Runner, directory string, arguments ...string) (string, error) {
	content, err := git(ctx, runner, directory, arguments...)
	return strings.TrimSpace(string(content)), err
}

func physicalPath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(absolute))
	if err != nil {
		return "", err
	}
	return resolved, nil
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
