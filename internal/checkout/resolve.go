package checkout

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

func Resolve(ctx context.Context, layout state.Layout, loaded *manifest.Loaded, service *manifest.Service, runner maintenance.Runner) (Roots, error) {
	if runner == nil {
		runner = maintenance.CommandRunner{}
	}
	declaredRoot, err := manifest.ServiceRepositoryRoot(&loaded.Manifest, loaded.WorkspaceRoot, service)
	if err != nil {
		return Roots{}, err
	}
	selectedPath, selected, err := storedPath(layout, service.Name)
	if err != nil {
		return Roots{}, err
	}
	if selected {
		repository, entries, err := repositoryWorktrees(ctx, loaded, service, runner)
		if err != nil {
			return Roots{}, err
		}
		matched, err := matchWorktree(selectedPath, repository.Primary, entries)
		if err != nil {
			return Roots{}, err
		}
		return selectedRoots(service, repository.Name, matched, true)
	}
	declaredWorking, err := manifest.ServiceWorkingDirectory(&loaded.Manifest, loaded.WorkspaceRoot, service)
	if err != nil {
		return Roots{}, err
	}
	return Roots{
		Service:          service.Name,
		Repository:       serviceRepository(service),
		RepositoryRoot:   declaredRoot,
		WorkingDirectory: declaredWorking,
	}, nil
}

func selectedRoots(service *manifest.Service, repository string, entry Worktree, selected bool) (Roots, error) {
	working, err := filepath.Abs(filepath.Join(entry.Path, service.WorkingDirectory))
	if err != nil {
		return Roots{}, errs.Wrap(errs.ExitConflict, "RG1706", "resolve selected service working directory", err)
	}
	working, err = physicalPath(working)
	if err != nil {
		return Roots{}, errs.Wrap(errs.ExitConflict, "RG1706", "resolve selected service working directory", err)
	}
	if !within(entry.Path, working) {
		return Roots{}, errs.New(errs.ExitConflict, "RG1707", "selected service working directory resolves outside its worktree")
	}
	info, statErr := os.Stat(working)
	if statErr != nil || !info.IsDir() {
		return Roots{}, errs.New(errs.ExitConflict, "RG1708", "selected service working directory is not a directory")
	}
	return Roots{
		Service:          service.Name,
		Repository:       repository,
		RepositoryRoot:   entry.Path,
		WorkingDirectory: working,
		Branch:           entry.Branch,
		HeadOID:          entry.Head,
		Primary:          entry.Primary,
		Selected:         selected,
		SelectedPath:     entry.Path,
	}, nil
}

func repositoryWorktrees(ctx context.Context, loaded *manifest.Loaded, service *manifest.Service, runner maintenance.Runner) (maintenance.Repository, []Worktree, error) {
	repositories, failures := maintenance.Discover(ctx, loaded, []string{serviceRepository(service)}, runner)
	if len(repositories) == 0 {
		detail := "service repository is not a Git worktree"
		if len(failures) > 0 {
			detail = failures[0].Error
		}
		return maintenance.Repository{}, nil, errs.New(errs.ExitConflict, "RG1709", detail)
	}
	repository := repositories[0]
	if len(repositories) > 1 {
		declared, err := manifest.ServiceWorkingDirectory(&loaded.Manifest, loaded.WorkspaceRoot, service)
		if err != nil {
			return maintenance.Repository{}, nil, err
		}
		declared, err = physicalPath(declared)
		if err != nil {
			return maintenance.Repository{}, nil, err
		}
		found := false
		for _, candidate := range repositories {
			if within(candidate.TopLevel, declared) || within(candidate.Path, declared) {
				repository = candidate
				found = true
				break
			}
		}
		if !found {
			return maintenance.Repository{}, nil, errs.New(errs.ExitConflict, "RG1709", "service repository is not a Git worktree")
		}
	}
	entries, err := listWorktrees(ctx, runner, repository.TopLevel)
	if err != nil {
		return maintenance.Repository{}, nil, errs.Wrap(errs.ExitConflict, "RG1710", "list registered worktrees", err)
	}
	return repository, entries, nil
}

func matchWorktree(selector, primary string, entries []Worktree) (Worktree, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return Worktree{}, errs.New(errs.ExitUsage, "RG1711", "worktree selector is required")
	}
	if strings.EqualFold(selector, "primary") {
		return requireAttached(findPrimary(primary, entries))
	}
	if resolved, err := physicalPath(selector); err == nil {
		for _, entry := range entries {
			if entry.Path == resolved {
				return requireAttached(entry)
			}
		}
	}
	var matches []Worktree
	for _, entry := range entries {
		if entry.Branch == selector || filepath.Base(entry.Path) == selector {
			matches = append(matches, entry)
		}
	}
	if len(matches) == 1 {
		return requireAttached(matches[0])
	}
	if len(matches) > 1 {
		return Worktree{}, errs.New(errs.ExitUsage, "RG1712", "worktree selector is ambiguous")
	}
	return Worktree{}, errs.New(errs.ExitConflict, "RG1713", "selector is not a registered worktree of this service repository")
}

func findPrimary(primary string, entries []Worktree) Worktree {
	for _, entry := range entries {
		if entry.Primary || (primary != "" && entry.Path == primary) {
			return entry
		}
	}
	if len(entries) > 0 {
		return entries[0]
	}
	return Worktree{}
}

func requireAttached(entry Worktree) (Worktree, error) {
	if entry.Path == "" {
		return Worktree{}, errs.New(errs.ExitConflict, "RG1713", "selector is not a registered worktree of this service repository")
	}
	if entry.Detached {
		return Worktree{}, errs.New(errs.ExitConflict, "RG1714", "refusing a detached worktree")
	}
	return entry, nil
}

func serviceRepository(service *manifest.Service) string {
	if service.Repository == "" {
		return manifest.WorkspaceRepository
	}
	return service.Repository
}
