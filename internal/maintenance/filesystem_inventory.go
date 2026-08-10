package maintenance

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DiscoverFilesystem returns one repository per physical Git common directory.
// A target inside Git protects that exact worktree and does not scan siblings.
func DiscoverFilesystem(ctx context.Context, target string, includeSubmodules bool, runner Runner) ([]Repository, []Failure) {
	if runner == nil {
		runner = CommandRunner{}
	}
	resolved, err := physicalPath(target)
	if err != nil {
		return nil, []Failure{{Operation: "resolve-target", Path: target, Error: err.Error()}}
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		if err == nil {
			err = fmt.Errorf("target is not a directory")
		}
		return nil, []Failure{{Operation: "inspect-target", Path: resolved, Error: err.Error()}}
	}
	if top, topErr := gitText(ctx, runner, resolved, "rev-parse", "--show-toplevel"); topErr == nil {
		repository, discoverErr := discoverFilesystemCandidate(ctx, runner, top, resolved, includeSubmodules, true)
		if discoverErr != nil {
			return nil, []Failure{{Operation: "discover", Path: resolved, Error: discoverErr.Error()}}
		}
		if repository.CommonDir == "" {
			return nil, nil
		}
		return []Repository{repository}, nil
	}

	var candidates []string
	var failures []Failure
	walkErr := filepath.WalkDir(resolved, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			failures = append(failures, Failure{Operation: "scan", Path: path, Error: walkErr.Error()})
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Name() != ".git" {
			return nil
		}
		candidates = append(candidates, filepath.Dir(path))
		if entry.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if walkErr != nil {
		failures = append(failures, Failure{Operation: "scan", Path: resolved, Error: walkErr.Error()})
	}

	byCommon := make(map[string]Repository)
	for _, candidate := range candidates {
		repository, discoverErr := discoverFilesystemCandidate(ctx, runner, candidate, resolved, includeSubmodules, false)
		if discoverErr != nil {
			failures = append(failures, Failure{Operation: "discover", Path: candidate, Error: discoverErr.Error()})
			continue
		}
		if repository.CommonDir == "" {
			continue
		}
		if _, exists := byCommon[repository.CommonDir]; !exists {
			byCommon[repository.CommonDir] = repository
		}
	}
	repositories := make([]Repository, 0, len(byCommon))
	for _, repository := range byCommon {
		repositories = append(repositories, repository)
	}
	sort.Slice(repositories, func(i, j int) bool { return repositories[i].TopLevel < repositories[j].TopLevel })
	return repositories, failures
}

func discoverFilesystemCandidate(ctx context.Context, runner Runner, candidate, scanRoot string, includeSubmodules, protect bool) (Repository, error) {
	top, err := gitText(ctx, runner, candidate, "rev-parse", "--show-toplevel")
	if err != nil {
		return Repository{}, fmt.Errorf("path is not a Git worktree")
	}
	top, err = physicalPath(top)
	if err != nil {
		return Repository{}, fmt.Errorf("resolve Git top-level: %w", err)
	}
	superproject, superErr := gitText(ctx, runner, top, "rev-parse", "--show-superproject-working-tree")
	if superErr == nil && superproject != "" && !includeSubmodules {
		return Repository{}, nil
	}
	common, err := gitText(ctx, runner, top, "rev-parse", "--git-common-dir")
	if err != nil {
		return Repository{}, fmt.Errorf("discover Git common directory: %w", err)
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(top, common)
	}
	common, err = physicalPath(common)
	if err != nil {
		return Repository{}, fmt.Errorf("resolve Git common directory: %w", err)
	}
	worktrees, err := listWorktrees(ctx, runner, top)
	if err != nil || len(worktrees) == 0 {
		return Repository{}, fmt.Errorf("discover primary worktree: %w", err)
	}
	primary, err := physicalPath(worktrees[0].Path)
	if err != nil {
		return Repository{}, fmt.Errorf("resolve primary worktree: %w", err)
	}
	remoteURL, err := gitText(ctx, runner, primary, "remote", "get-url", "origin")
	if err != nil {
		return Repository{}, fmt.Errorf("remote %q is unavailable", "origin")
	}
	name := filepath.Base(primary)
	if relative, relativeErr := filepath.Rel(scanRoot, primary); !protect && relativeErr == nil && relative != "." &&
		relative != ".." && !filepath.IsAbs(relative) && !strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		name = filepath.ToSlash(relative)
	}
	repository := Repository{
		Name: name, Path: primary, TopLevel: primary, CommonDir: common,
		Primary: primary, Remote: "origin", RemoteSlug: githubSlug(remoteURL),
	}
	if protect && top != primary {
		repository.DeclaredPaths = []string{top}
	}
	return repository, nil
}
