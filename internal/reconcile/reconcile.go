package reconcile

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
)

func Run(ctx context.Context, options Options) (Report, error) {
	started := time.Now().UTC()
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Runner == nil {
		options.Runner = maintenance.CommandRunner{}
	}
	if options.Coordinator == nil {
		options.Coordinator = maintenance.NoopCoordinator{}
	}
	if options.RecoveryTimeout <= 0 {
		options.RecoveryTimeout = 2 * time.Minute
	}
	target, err := filepath.Abs(options.Target)
	if err != nil {
		return Report{}, errs.Wrap(errs.ExitUsage, "RG1631", "resolve reconciliation target", err)
	}
	report := Report{
		Operation: "reconcile", Target: target, DryRun: options.DryRun,
		StartedAt:    started.Format(time.RFC3339Nano),
		Repositories: make([]RepositoryResult, 0), Failures: make([]maintenance.Failure, 0),
	}
	repositories, discoveryFailures := maintenance.DiscoverFilesystem(ctx, target, options.IncludeSubmodules, options.Runner)
	report.Failures = append(report.Failures, discoveryFailures...)
	coordinator := processGuardCoordinator{delegate: options.Coordinator, runner: options.Runner}
	for _, repository := range repositories {
		result := RepositoryResult{Name: repository.Name, Path: repository.Primary, CommonDir: repository.CommonDir}
		syncReport, _ := maintenance.SyncRepositories(ctx, []maintenance.Repository{repository}, options.DryRun, options.RecoveryTimeout, options.Runner, coordinator)
		syncFailed := appendSyncResult(&result, &report, syncReport)
		if result.DefaultBranch != "" {
			state, inspectErr := inspectRoot(ctx, repository, options.Runner)
			if inspectErr != nil {
				result.Root = RootResult{Path: repository.Primary, Action: "preserved", Reason: "inspection-failed", Detail: inspectErr.Error()}
				report.Failures = append(report.Failures, failure(repository, "inspect-primary", repository.Primary, inspectErr))
			} else if rootRecoveryEligible(result.Sync, state, options.DryRun) {
				root, recoveryErr := recoverRoot(ctx, repository, result.DefaultBranch, result.Sync, state, options)
				result.Root = root
				if recoveryErr != nil {
					report.Failures = append(report.Failures, failure(repository, "recover-primary", repository.Primary, recoveryErr))
				}
				if recoveryErr == nil && root.Action == "stashed" {
					retry, _ := maintenance.SyncRepositories(ctx, []maintenance.Repository{repository}, false, options.RecoveryTimeout, options.Runner, coordinator)
					syncFailed = appendSyncResult(&result, &report, retry) || syncFailed
				}
			} else {
				result.Root = state.result()
				result.Root.Reason = "default-not-current"
				result.Root.Detail = "default branch state is " + result.Sync.State
			}
			if syncErr := requiredSyncFailure(result.Sync, result.Root, options.DryRun); syncErr != nil && !syncFailed {
				report.Failures = append(report.Failures, failure(repository, "synchronize-default", result.Sync.Path, syncErr))
			}
			pruneReport, _ := maintenance.PruneRepositories(ctx, []maintenance.Repository{repository}, options.DryRun, options.Runner)
			if len(pruneReport.Repositories) != 0 {
				result.Worktrees = pruneReport.Repositories[0].Worktrees
			}
			report.Failures = append(report.Failures, pruneReport.Failures...)
		} else {
			result.Root = RootResult{Path: repository.Primary, Action: "preserved", Reason: "default-unavailable"}
		}
		if result.Worktrees == nil {
			result.Worktrees = make([]maintenance.WorktreeDecision, 0)
		}
		report.Repositories = append(report.Repositories, result)
	}
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if len(report.Failures) != 0 {
		return report, errs.New(errs.ExitPartial, "RG1630", fmt.Sprintf("repository reconciliation completed with %d failure(s)", len(report.Failures)))
	}
	return report, nil
}

func rootRecoveryEligible(sync maintenance.SyncRepository, state rootState, dryRun bool) bool {
	if sync.State == "current" {
		return true
	}
	if sync.State == "dirty" {
		return state.branch == sync.DefaultBranch
	}
	return dryRun && (sync.State == "behind" || sync.State == "remote-object-unavailable")
}

func appendSyncResult(result *RepositoryResult, report *Report, syncReport maintenance.SyncReport) bool {
	if len(syncReport.Repositories) != 0 {
		result.Sync = syncReport.Repositories[0]
		result.DefaultBranch = result.Sync.DefaultBranch
	}
	report.Failures = append(report.Failures, syncReport.Failures...)
	return len(syncReport.Failures) != 0
}

func requiredSyncFailure(sync maintenance.SyncRepository, root RootResult, dryRun bool) error {
	if dryRun {
		switch sync.State {
		case "current", "behind", "remote-object-unavailable":
			return nil
		case "dirty":
			if root.Action == "would-stash" {
				return nil
			}
		}
	} else if sync.State == "current" {
		return nil
	}
	if sync.State == "" {
		return fmt.Errorf("default branch state is unavailable")
	}
	return fmt.Errorf("default branch remains %s", sync.State)
}

func failure(repository maintenance.Repository, operation, path string, err error) maintenance.Failure {
	return maintenance.Failure{Repository: repository.Name, Operation: operation, Path: path, Error: err.Error()}
}
