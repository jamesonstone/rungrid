package reconcile

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

func recoverRoot(ctx context.Context, repository maintenance.Repository, defaultBranch string, syncState maintenance.SyncRepository, state rootState, options Options) (RootResult, error) {
	result := state.result()
	now := options.Now()
	if state.branch == "" {
		result.Reason = "detached-primary"
		return result, nil
	}
	if state.branch == defaultBranch {
		return recoverDirtyDefault(ctx, repository, syncState, state, result, options)
	}
	stale := inactiveFor(state, now, inactivityThreshold)
	if state.dirty() {
		if !stale {
			result.Reason = "recent-dirty-feature-root"
			return result, nil
		}
		if err := requireRootProcessProof(ctx, state, &result, options); err != nil || result.Reason == "primary-in-use" {
			return result, err
		}
		issueTitle, paths, err := validateWIP(ctx, repository, state, options.Runner)
		if err != nil {
			result.Reason, result.Detail = "wip-validation-failed", err.Error()
			return result, err
		}
		if options.DryRun {
			result.Action, result.Reason = "would-commit-and-switch", "stale-dirty-gh-root"
			return result, nil
		}
		paused, err := pauseRoot(ctx, repository, state, options)
		if err != nil {
			result.Reason, result.Detail = "service-pause-failed", err.Error()
			return result, err
		}
		result.Services, result.OwnedProcessIDs = paused.services, paused.owned
		commitOID, err := commitWIP(ctx, repository, paused.state, issueTitle, paths, options.Runner)
		if err != nil {
			err = finishRootMutation(paused, options.RecoveryTimeout, err)
			result.Reason, result.Detail = "wip-commit-failed", err.Error()
			return result, err
		}
		committedState, inspectErr := inspectRoot(ctx, repository, options.Runner)
		if inspectErr != nil || committedState.branch != state.branch || committedState.headOID != commitOID ||
			committedState.dirty() || committedState.processErr != nil || len(withoutStrings(committedState.processes, paused.owned)) != 0 {
			err := finishRootMutation(paused, options.RecoveryTimeout, fmt.Errorf("WIP commit state could not be revalidated: %v", inspectErr))
			result.Action, result.Reason, result.Detail = "committed", "wip-revalidation-failed", err.Error()
			result.WIPCommitOID = commitOID
			return result, err
		}
		if err := switchPrimary(ctx, repository, committedState, defaultBranch, syncState.RemoteOID, paused.owned, options.Runner); err != nil {
			err = finishRootMutation(paused, options.RecoveryTimeout, err)
			result.Action, result.Reason, result.Detail = "committed", "default-switch-failed", err.Error()
			result.WIPCommitOID = commitOID
			return result, err
		}
		result.Action, result.Reason, result.WIPCommitOID = "committed-and-switched", "stale-dirty-gh-root", commitOID
		if err := finishRootMutation(paused, options.RecoveryTimeout, nil); err != nil {
			result.Reason, result.Detail = "service-resume-failed", err.Error()
			return result, err
		}
		return result, nil
	}

	merged, mergeErr := exactMergedPR(ctx, repository, state.branch, defaultBranch, state.headOID, options.Runner)
	if !stale && !merged {
		result.Reason = "recent-clean-feature-root"
		if mergeErr != nil {
			result.Detail = mergeErr.Error()
		}
		return result, nil
	}
	if err := requireRootProcessProof(ctx, state, &result, options); err != nil || result.Reason == "primary-in-use" {
		return result, err
	}
	if options.DryRun {
		result.Action = "would-switch"
		if merged {
			result.Reason = "merged-feature-root"
		} else {
			result.Reason = "stale-clean-feature-root"
		}
		return result, nil
	}
	paused, err := pauseRoot(ctx, repository, state, options)
	if err != nil {
		result.Reason, result.Detail = "service-pause-failed", err.Error()
		return result, err
	}
	result.Services, result.OwnedProcessIDs = paused.services, paused.owned
	if err := switchPrimary(ctx, repository, paused.state, defaultBranch, syncState.RemoteOID, paused.owned, options.Runner); err != nil {
		err = finishRootMutation(paused, options.RecoveryTimeout, err)
		result.Reason, result.Detail = "default-switch-failed", err.Error()
		return result, err
	}
	result.Action = "switched"
	if merged {
		result.Reason = "merged-feature-root"
	} else {
		result.Reason = "stale-clean-feature-root"
	}
	if err := finishRootMutation(paused, options.RecoveryTimeout, nil); err != nil {
		result.Reason, result.Detail = "service-resume-failed", err.Error()
		return result, err
	}
	return result, nil
}

func recoverDirtyDefault(ctx context.Context, repository maintenance.Repository, syncState maintenance.SyncRepository, state rootState, result RootResult, options Options) (RootResult, error) {
	if !state.dirty() {
		result.Action, result.Reason = "none", "default-root-clean"
		return result, nil
	}
	if syncState.State == "current" {
		result.Reason = "dirty-default-current"
		return result, nil
	}
	if syncState.State != "dirty" && syncState.State != "behind" {
		result.Reason = "dirty-default-not-behind"
		return result, nil
	}
	if !inactiveFor(state, options.Now(), inactivityThreshold) {
		result.Reason = "recent-dirty-default"
		return result, nil
	}
	if err := requireRootProcessProof(ctx, state, &result, options); err != nil {
		return result, err
	}
	if result.Reason == "primary-in-use" {
		result.Reason = "dirty-default-in-use"
		return result, nil
	}
	if options.DryRun {
		result.Action, result.Reason = "would-stash", "stale-dirty-default"
		return result, nil
	}
	paused, err := pauseRoot(ctx, repository, state, options)
	if err != nil {
		result.Reason, result.Detail = "service-pause-failed", err.Error()
		return result, err
	}
	result.Services, result.OwnedProcessIDs = paused.services, paused.owned
	stashOID, err := stashPrimary(ctx, repository, paused.state, options.Now(), paused.owned, options.Runner)
	if err != nil {
		err = finishRootMutation(paused, options.RecoveryTimeout, err)
		result.Reason, result.Detail, result.StashOID = "stash-failed", err.Error(), stashOID
		if stashOID != "" {
			result.Action = "stashed"
		}
		return result, err
	}
	result.Action, result.Reason, result.StashOID = "stashed", "stale-dirty-default", stashOID
	if err := finishRootMutation(paused, options.RecoveryTimeout, nil); err != nil {
		result.Reason, result.Detail = "service-resume-failed", err.Error()
		return result, err
	}
	return result, nil
}

func inactiveFor(state rootState, now time.Time, threshold time.Duration) bool {
	return !state.activityAt.IsZero() && !now.Before(state.activityAt) && now.Sub(state.activityAt) >= threshold
}

func switchPrimary(ctx context.Context, repository maintenance.Repository, expected rootState, defaultBranch, expectedDefaultOID string, owned []string, runner maintenance.Runner) error {
	fresh, err := inspectRoot(ctx, repository, runner)
	if err != nil {
		return fmt.Errorf("revalidate primary root: %w", err)
	}
	if !sameRootState(fresh, expected) {
		return fmt.Errorf("primary root changed before branch switch")
	}
	if fresh.processErr != nil || len(withoutStrings(fresh.processes, owned)) != 0 {
		return fmt.Errorf("primary root became active before branch switch")
	}
	defaultOID, err := gitText(ctx, runner, repository.Primary, "rev-parse", "--verify", "refs/heads/"+defaultBranch)
	if err != nil || expectedDefaultOID == "" || defaultOID != expectedDefaultOID {
		return fmt.Errorf("default branch changed before branch switch")
	}
	if _, err := runner.Run(ctx, repository.Primary, "git", "switch", defaultBranch); err != nil {
		return err
	}
	branch, err := gitText(ctx, runner, repository.Primary, "symbolic-ref", "--quiet", "--short", "HEAD")
	updatedOID, oidErr := gitText(ctx, runner, repository.Primary, "rev-parse", "HEAD")
	if err != nil || branch != defaultBranch || oidErr != nil || updatedOID != expectedDefaultOID {
		return fmt.Errorf("primary root did not switch to expected default branch")
	}
	return nil
}

func stashPrimary(ctx context.Context, repository maintenance.Repository, expected rootState, now time.Time, owned []string, runner maintenance.Runner) (string, error) {
	fresh, err := inspectRoot(ctx, repository, runner)
	if err != nil || !sameRootState(fresh, expected) {
		return "", fmt.Errorf("primary root changed before stash")
	}
	if fresh.processErr != nil || len(withoutStrings(fresh.processes, owned)) != 0 {
		return "", fmt.Errorf("primary root became active before stash")
	}
	before, _ := gitText(ctx, runner, repository.Primary, "rev-parse", "--verify", "refs/stash")
	label := fmt.Sprintf("rungrid reconcile %s %s", now.UTC().Format("20060102T150405.000000000Z"), repository.Name)
	if _, err := runner.Run(ctx, repository.Primary, "git", "stash", "push", "--include-untracked", "-m", label, "--"); err != nil {
		return "", err
	}
	after, err := gitText(ctx, runner, repository.Primary, "rev-parse", "--verify", "refs/stash")
	if err != nil || after == "" || after == before {
		return "", fmt.Errorf("stash OID was not created")
	}
	parents, parentErr := gitText(ctx, runner, repository.Primary, "show", "-s", "--format=%P", after)
	subject, subjectErr := gitText(ctx, runner, repository.Primary, "show", "-s", "--format=%s", after)
	if parentErr != nil || subjectErr != nil || !strings.HasPrefix(parents, expected.headOID+" ") || !strings.HasSuffix(subject, label) {
		return after, fmt.Errorf("created stash OID could not be tied to the expected root state")
	}
	remaining, err := gitPaths(ctx, runner, repository.Primary, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil || len(remaining) != 0 {
		return after, fmt.Errorf("primary root is not clean after stash")
	}
	return after, nil
}

func sanitizeTitle(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
