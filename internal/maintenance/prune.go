package maintenance

import (
	"context"
	"fmt"
	"sort"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
)

func Prune(ctx context.Context, loaded *manifest.Loaded, options Options, runner Runner) (PruneReport, error) {
	report := PruneReport{Operation: OperationPrune, DryRun: options.DryRun, StartedAt: timestamp()}
	if runner == nil {
		runner = CommandRunner{}
	}
	repositories, failures := Discover(ctx, loaded, options.Repositories, runner)
	report.Failures = append(report.Failures, failures...)
	for _, repository := range repositories {
		result, resultFailures := pruneRepository(ctx, repository, options.DryRun, runner)
		report.Repositories = append(report.Repositories, result)
		report.Failures = append(report.Failures, resultFailures...)
	}
	report.FinishedAt = timestamp()
	if len(report.Failures) != 0 {
		return report, errs.New(errs.ExitPartial, "RG1602", fmt.Sprintf("worktree prune completed with %d failure(s)", len(report.Failures)))
	}
	return report, nil
}

// PruneRepositories applies the native worktree proof to an explicit physical
// repository inventory. It does not add an interactive confirmation layer.
func PruneRepositories(ctx context.Context, repositories []Repository, dryRun bool, runner Runner) (PruneReport, error) {
	report := PruneReport{Operation: OperationPrune, DryRun: dryRun, StartedAt: timestamp()}
	if runner == nil {
		runner = CommandRunner{}
	}
	for _, repository := range repositories {
		result, failures := pruneRepository(ctx, repository, dryRun, runner)
		report.Repositories = append(report.Repositories, result)
		report.Failures = append(report.Failures, failures...)
	}
	report.FinishedAt = timestamp()
	if len(report.Failures) != 0 {
		return report, errs.New(errs.ExitPartial, "RG1602", fmt.Sprintf("worktree prune completed with %d failure(s)", len(report.Failures)))
	}
	return report, nil
}

func pruneRepository(ctx context.Context, repository Repository, dryRun bool, runner Runner) (PruneResult, []Failure) {
	result := PruneResult{Name: repository.Name, Aliases: repository.Aliases, Remote: repository.Remote}
	branch, _, err := liveDefault(ctx, runner, repository)
	if err != nil {
		return result, []Failure{{Repository: repository.Name, Operation: "discover-default", Error: err.Error()}}
	}
	result.DefaultBranch = branch
	if !dryRun {
		if _, err := git(ctx, runner, repository.TopLevel, "fetch", "--prune", repository.Remote); err != nil {
			return result, []Failure{{Repository: repository.Name, Operation: "fetch", Error: err.Error()}}
		}
		branch, _, err = liveDefault(ctx, runner, repository)
		if err != nil {
			return result, []Failure{{Repository: repository.Name, Operation: "rediscover-default", Error: err.Error()}}
		}
		result.DefaultBranch = branch
	}
	entries, err := listWorktrees(ctx, runner, repository.TopLevel)
	if err != nil {
		return result, []Failure{{Repository: repository.Name, Operation: "list-worktrees", Error: err.Error()}}
	}
	var failures []Failure
	for index, entry := range entries {
		decision, decisionErr := evaluateWorktree(ctx, runner, repository, branch, entry, index == 0)
		if decisionErr != nil {
			failures = append(failures, Failure{Repository: repository.Name, Operation: "inspect-worktree", Path: entry.Path, Error: decisionErr.Error()})
		}
		if dryRun && decision.Action == "remove" {
			decision.Action = "would-remove"
		}
		result.Worktrees = append(result.Worktrees, decision)
	}
	if dryRun {
		return result, failures
	}
	for index := range result.Worktrees {
		decision := &result.Worktrees[index]
		if decision.Action != "remove" {
			continue
		}
		freshBranch, _, defaultErr := liveDefault(ctx, runner, repository)
		if defaultErr != nil || freshBranch != branch {
			decision.Action, decision.Reason = "preserved", "default-branch-revalidation-failed"
			decision.Detail = "remote default branch changed during prune"
			if defaultErr != nil {
				decision.Detail = defaultErr.Error()
			}
			failures = append(failures, Failure{Repository: repository.Name, Operation: "revalidate-default", Path: decision.Path, Error: decision.Detail})
			continue
		}
		freshEntries, listErr := listWorktrees(ctx, runner, repository.TopLevel)
		if listErr != nil {
			decision.Action, decision.Reason, decision.Detail = "preserved", "revalidation-failed", listErr.Error()
			failures = append(failures, Failure{Repository: repository.Name, Operation: "revalidate-worktree", Path: decision.Path, Error: decision.Detail})
			continue
		}
		entry, registered := findWorktree(freshEntries, decision.Path)
		if !registered {
			decision.Action, decision.Reason, decision.Detail = "gone", "worktree-no-longer-registered", ""
			continue
		}
		fresh, freshErr := evaluateWorktree(ctx, runner, repository, branch, entry, false)
		if freshErr != nil {
			fresh.Action, fresh.Reason, fresh.Detail = "preserved", "revalidation-failed", freshErr.Error()
			failures = append(failures, Failure{Repository: repository.Name, Operation: "revalidate-worktree", Path: decision.Path, Error: fresh.Detail})
			*decision = fresh
			continue
		}
		if fresh.Action != "remove" {
			*decision = fresh
			continue
		}
		removed, removalErr := removeWorktree(ctx, runner, repository, branch, fresh)
		fresh.Action = "removed"
		if removalErr != nil {
			fresh.Action, fresh.Reason = "failed", "worktree-removal-failed"
			failureOperation := "remove-worktree"
			if removed {
				fresh.Action, fresh.Reason = "removed", "worktree-removed-local-branch-preserved"
				failureOperation = "delete-local-branch"
			}
			fresh.Detail = removalErr.Error()
			failures = append(failures, Failure{Repository: repository.Name, Operation: failureOperation, Path: fresh.Path, Error: removalErr.Error()})
		}
		*decision = fresh
	}
	if _, err := git(ctx, runner, repository.TopLevel, "worktree", "prune", "--expire", "now"); err != nil {
		failures = append(failures, Failure{Repository: repository.Name, Operation: "prune-metadata", Error: err.Error()})
	}
	sort.Slice(result.Worktrees, func(i, j int) bool { return result.Worktrees[i].Path < result.Worktrees[j].Path })
	return result, failures
}

func findWorktree(entries []worktree, path string) (worktree, bool) {
	for _, entry := range entries {
		if entry.Path == path {
			return entry, true
		}
	}
	return worktree{}, false
}
