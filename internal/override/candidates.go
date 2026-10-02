package override

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

type worktree struct {
	path   string
	head   string
	branch string
}

// Candidates lists every registered worktree of the target's repository for
// the picker, most recently updated first. The current checkout is the active
// override, or the original checkout when none is set.
func Candidates(ctx context.Context, loaded *manifest.Loaded, layout state.Layout, generationID, target string, runner maintenance.Runner) (Repository, []Candidate, error) {
	runner = defaultRunner(runner)
	repository, _, err := FindTarget(loaded, Repositories(ctx, loaded, runner), target)
	if err != nil {
		return Repository{}, nil, err
	}
	current := repository.TopLevel
	if entries, loadErr := Load(layout, generationID); loadErr == nil {
		if entry, exists := entries[repository.Name]; exists {
			current = entry.Path
		}
	}
	worktrees, err := listWorktrees(ctx, runner, repository.TopLevel)
	if err != nil {
		return Repository{}, nil, errs.Wrap(errs.ExitConflict, "RG1809", "list worktrees of "+repository.Name, err)
	}
	result := make([]Candidate, 0, len(worktrees))
	for index, item := range worktrees {
		result = append(result, describe(ctx, runner, item, index == 0, current))
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].UpdatedAt.After(result[j].UpdatedAt) })
	return repository, result, nil
}

func describe(ctx context.Context, runner maintenance.Runner, item worktree, primary bool, current string) Candidate {
	candidate := Candidate{Path: item.path, Branch: item.branch, HeadOID: item.head, Primary: primary, Current: item.path == current}
	gitDirectory, err := gitText(ctx, runner, item.path, "rev-parse", "--absolute-git-dir")
	if err != nil {
		return candidate
	}
	if primary {
		candidate.CreatedAt = modified(filepath.Join(gitDirectory, "description"), gitDirectory)
	} else {
		candidate.CreatedAt = modified(filepath.Join(gitDirectory, "commondir"), gitDirectory)
	}
	fields := strings.SplitN(strings.TrimSpace(mustGit(ctx, runner, item.path, "log", "-1", "--format=%s%x00%cI")), "\x00", 2)
	candidate.Subject = fields[0]
	committed := time.Time{}
	if len(fields) == 2 {
		committed, _ = time.Parse(time.RFC3339, fields[1])
	}
	candidate.UpdatedAt = modified(filepath.Join(gitDirectory, "logs", "HEAD"), "")
	if candidate.UpdatedAt.IsZero() || committed.After(candidate.UpdatedAt) {
		candidate.UpdatedAt = committed
	}
	candidate.Dirty = mustGit(ctx, runner, item.path, "status", "--porcelain") != ""
	candidate.CreatedAt, candidate.UpdatedAt = candidate.CreatedAt.UTC(), candidate.UpdatedAt.UTC()
	return candidate
}

func modified(path, fallback string) time.Time {
	for _, candidate := range []string{path, fallback} {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil {
			return info.ModTime()
		}
	}
	return time.Time{}
}

func mustGit(ctx context.Context, runner maintenance.Runner, directory string, arguments ...string) string {
	value, err := gitText(ctx, runner, directory, arguments...)
	if err != nil {
		return ""
	}
	return value
}

func listWorktrees(ctx context.Context, runner maintenance.Runner, directory string) ([]worktree, error) {
	content, err := defaultRunner(runner).Run(ctx, directory, "git", "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var result []worktree
	for _, line := range strings.Split(string(content), "\n") {
		switch {
		case strings.HasPrefix(line, "worktree "):
			path, pathErr := physicalPath(strings.TrimPrefix(line, "worktree "))
			if pathErr != nil {
				path = strings.TrimPrefix(line, "worktree ")
			}
			result = append(result, worktree{path: path})
		case len(result) == 0:
		case strings.HasPrefix(line, "HEAD "):
			result[len(result)-1].head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch refs/heads/"):
			result[len(result)-1].branch = strings.TrimPrefix(line, "branch refs/heads/")
		}
	}
	return result, nil
}
