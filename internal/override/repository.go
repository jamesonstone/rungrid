package override

import (
	"context"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
)

// Repositories groups the manifest's services by the Git top level of their
// declared working directories. Services whose working directory is not inside
// a Git checkout belong to no repository and cannot be overridden.
func Repositories(ctx context.Context, loaded *manifest.Loaded, runner maintenance.Runner) []Repository {
	runner = defaultRunner(runner)
	byTop := map[string]*Repository{}
	var order []string
	for index := range loaded.Manifest.Services {
		service := &loaded.Manifest.Services[index]
		top, err := serviceTopLevel(ctx, loaded, service, runner)
		if err != nil {
			continue
		}
		repository, exists := byTop[top]
		if !exists {
			repository = &Repository{TopLevel: top, Name: repositoryName(loaded, service, top)}
			repository.CommonDir, _ = commonDir(ctx, runner, top)
			remote := manifest.RepositoryConfiguration(&loaded.Manifest, service.Repository).Remote
			if remote == "" {
				remote = "origin"
			}
			repository.Origin, _ = gitText(ctx, runner, top, "remote", "get-url", remote)
			byTop[top] = repository
			order = append(order, top)
		}
		if service.Source == "external" {
			repository.External = append(repository.External, service.Name)
		} else {
			repository.Managed = append(repository.Managed, service.Name)
		}
	}
	result := make([]Repository, 0, len(order))
	for _, top := range order {
		result = append(result, *byTop[top])
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

// FindTarget resolves a repository or service name. A service resolves to the
// repository containing its declared working directory; the returned service
// name is empty when target named a repository.
func FindTarget(loaded *manifest.Loaded, repositories []Repository, target string) (Repository, string, error) {
	target = strings.TrimSpace(target)
	if service, exists := manifest.FindService(&loaded.Manifest, target); exists {
		if service.Source == "external" {
			return Repository{}, "", errs.New(errs.ExitUsage, "RG1806", "Rungrid does not own external service "+target+"; it cannot be overridden")
		}
		for _, repository := range repositories {
			if contains(repository.Managed, target) {
				return repository, target, nil
			}
		}
		return Repository{}, "", errs.New(errs.ExitConflict, "RG1823", "service "+target+" does not run from a Git checkout")
	}
	for _, repository := range repositories {
		if repository.Name != target {
			continue
		}
		if len(repository.Managed) == 0 {
			return Repository{}, "", errs.New(errs.ExitUsage, "RG1807", "repository "+target+" has only external services; Rungrid manages none of them")
		}
		return repository, "", nil
	}
	return Repository{}, "", errs.New(errs.ExitUsage, "RG1805", "unknown service or repository: "+target)
}

func serviceTopLevel(ctx context.Context, loaded *manifest.Loaded, service *manifest.Service, runner maintenance.Runner) (string, error) {
	directory, err := manifest.ServiceWorkingDirectory(&loaded.Manifest, loaded.WorkspaceRoot, service)
	if err != nil {
		return "", err
	}
	return topLevel(ctx, runner, directory)
}

// repositoryName prefers the declared logical repository when its root is the
// Git top level, and otherwise names the checkout by its workspace-relative
// path, matching maintenance discovery for implicit-workspace manifests.
func repositoryName(loaded *manifest.Loaded, service *manifest.Service, top string) string {
	if service.Repository != "" && service.Repository != manifest.WorkspaceRepository {
		if root, err := manifest.ServiceRepositoryRoot(&loaded.Manifest, loaded.WorkspaceRoot, service); err == nil {
			if physical, physicalErr := physicalPath(root); physicalErr == nil && physical == top {
				return service.Repository
			}
		}
	}
	workspace, err := physicalPath(loaded.WorkspaceRoot)
	if err == nil {
		if relative, relErr := filepath.Rel(workspace, top); relErr == nil && !escapes(relative) {
			if relative == "." {
				return manifest.WorkspaceRepository
			}
			return filepath.ToSlash(relative)
		}
	}
	return filepath.Base(top)
}

func topLevel(ctx context.Context, runner maintenance.Runner, directory string) (string, error) {
	top, err := gitText(ctx, runner, directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return physicalPath(top)
}

func commonDir(ctx context.Context, runner maintenance.Runner, top string) (string, error) {
	common, err := gitText(ctx, runner, top, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(top, common)
	}
	return physicalPath(common)
}

func gitText(ctx context.Context, runner maintenance.Runner, directory string, arguments ...string) (string, error) {
	content, err := defaultRunner(runner).Run(ctx, directory, "git", arguments...)
	return strings.TrimSpace(string(content)), err
}

func defaultRunner(runner maintenance.Runner) maintenance.Runner {
	if runner == nil {
		return maintenance.CommandRunner{}
	}
	return runner
}

func physicalPath(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(filepath.Clean(absolute))
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
