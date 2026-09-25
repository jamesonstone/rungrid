package checkout

import (
	"context"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

func Use(ctx context.Context, layout state.Layout, loaded *manifest.Loaded, serviceName, selector string, runner maintenance.Runner) (UseReport, error) {
	service, exists := manifest.FindService(&loaded.Manifest, serviceName)
	if !exists {
		return UseReport{}, errs.New(errs.ExitUsage, "RG1715", "unknown service: "+serviceName)
	}
	if runner == nil {
		runner = maintenance.CommandRunner{}
	}
	repository, entries, err := repositoryWorktrees(ctx, loaded, service, runner)
	if err != nil {
		return UseReport{}, err
	}
	matched, err := matchWorktree(selector, repository.Primary, entries)
	if err != nil {
		return UseReport{}, err
	}
	store, err := loadStore(layout)
	if err != nil {
		return UseReport{}, err
	}
	store.Services[service.Name] = Selection{Path: matched.Path, SelectedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if err := saveStore(layout, store); err != nil {
		return UseReport{}, err
	}
	return UseReport{Operation: "worktrees-use", Service: service.Name, Path: matched.Path, Branch: matched.Branch, Action: "selected"}, nil
}

func Clear(layout state.Layout, loaded *manifest.Loaded, serviceName string) (UseReport, error) {
	if _, exists := manifest.FindService(&loaded.Manifest, serviceName); !exists {
		return UseReport{}, errs.New(errs.ExitUsage, "RG1715", "unknown service: "+serviceName)
	}
	store, err := loadStore(layout)
	if err != nil {
		return UseReport{}, err
	}
	delete(store.Services, serviceName)
	if err := saveStore(layout, store); err != nil {
		return UseReport{}, err
	}
	return UseReport{Operation: "worktrees-use", Service: serviceName, Action: "cleared", Detail: "service uses the declared repository root"}, nil
}

func Candidates(ctx context.Context, loaded *manifest.Loaded, serviceName string, runner maintenance.Runner) ([]Worktree, error) {
	service, exists := manifest.FindService(&loaded.Manifest, serviceName)
	if !exists {
		return nil, errs.New(errs.ExitUsage, "RG1715", "unknown service: "+serviceName)
	}
	_, entries, err := repositoryWorktrees(ctx, loaded, service, runner)
	return entries, err
}
