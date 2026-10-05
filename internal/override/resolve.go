package override

import (
	"context"
	"os"
	"path/filepath"

	"github.com/jamesonstone/rungrid/internal/checkout"
	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

// Execution is the effective checkout for one service. Override is nil when
// the service runs from its declared or selected checkout.
type Execution struct {
	checkout.Roots
	Override         *Entry
	originalWorkDir  string
	overrideMappings mapping
}

// Argv re-anchors relative path arguments for an overridden service and
// returns argv unchanged otherwise.
func (e Execution) Argv(argv []string) []string {
	if e.Override == nil {
		return append([]string(nil), argv...)
	}
	rewritten, _ := e.overrideMappings.rewriteArgv(argv, e.originalWorkDir)
	return rewritten
}

// Resolve returns where a service runs in generationID. An active override for
// the service's repository wins over a worktrees-use binding, which wins over
// the declared root. External services are never overridden.
func Resolve(ctx context.Context, layout state.Layout, generationID string, loaded *manifest.Loaded, service *manifest.Service, runner maintenance.Runner) (Execution, error) {
	if service.Source != "external" && generationID != "" {
		entries, err := Load(layout, generationID)
		if err != nil {
			return Execution{}, err
		}
		if len(entries) > 0 {
			execution, matched, err := resolveOverride(loaded, service, entries)
			if err != nil || matched {
				return execution, err
			}
		}
	}
	roots, err := checkout.Resolve(ctx, layout, loaded, service, runner)
	return Execution{Roots: roots}, err
}

func resolveOverride(loaded *manifest.Loaded, service *manifest.Service, entries map[string]Entry) (Execution, bool, error) {
	declared, err := declaredWorkingDirectory(loaded, service)
	if err != nil {
		return Execution{}, false, err
	}
	for _, candidate := range Sorted(entries) {
		if !contains(candidate.Services, service.Name) || !within(candidate.OriginalPath, declared) {
			continue
		}
		entry := candidate
		if info, statErr := os.Stat(filepath.Join(entry.Path, ".git")); statErr != nil || info == nil {
			return Execution{}, true, errs.New(errs.ExitConflict, "RG1821", "override checkout for "+entry.Repository+" is missing: "+entry.Path+"; run rungrid override clear "+entry.Repository)
		}
		view := mapping{original: entry.OriginalPath, override: entry.Path}
		working := view.mapPath(declared)
		if info, statErr := os.Stat(working); statErr != nil || !info.IsDir() {
			return Execution{}, true, errs.New(errs.ExitConflict, "RG1813", "service "+service.Name+" working directory does not exist in the override checkout: "+working)
		}
		root := executionRoot(loaded, service, view)
		return Execution{
			Roots: checkout.Roots{
				Service: service.Name, Repository: entry.Repository, RepositoryRoot: root,
				WorkingDirectory: working, Branch: entry.Branch, HeadOID: entry.HeadOID,
				Selected: true, SelectedPath: entry.Path,
			},
			Override: &entry, originalWorkDir: declared, overrideMappings: view,
		}, true, nil
	}
	return Execution{}, false, nil
}
