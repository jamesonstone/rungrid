package maintenance

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
)

func Sync(ctx context.Context, loaded *manifest.Loaded, options Options, runner Runner, coordinator Coordinator) (SyncReport, error) {
	report := SyncReport{Operation: OperationSync, DryRun: options.DryRun, StartedAt: timestamp()}
	if runner == nil {
		runner = CommandRunner{}
	}
	if coordinator == nil {
		coordinator = NoopCoordinator{}
	}
	repositories, failures := Discover(ctx, loaded, options.Repositories, runner)
	report.Failures = append(report.Failures, failures...)
	recoveryTimeout := loaded.Manifest.Runtime.StartupTimeout.Duration + loaded.Manifest.Runtime.ShutdownTimeout.Duration
	if recoveryTimeout <= 0 {
		recoveryTimeout = 2 * time.Minute
	}
	for _, repository := range repositories {
		result, resultFailures := syncRepository(ctx, repository, options.DryRun, recoveryTimeout, runner, coordinator)
		report.Repositories = append(report.Repositories, result)
		report.Failures = append(report.Failures, resultFailures...)
	}
	report.FinishedAt = timestamp()
	if len(report.Failures) != 0 {
		return report, errs.New(errs.ExitPartial, "RG1601", fmt.Sprintf("repository sync completed with %d failure(s)", len(report.Failures)))
	}
	return report, nil
}

// SyncRepositories applies the native synchronization contract to an explicit
// physical repository inventory. It is used by filesystem reconciliation;
// manifest-scoped callers continue to use Sync.
func SyncRepositories(ctx context.Context, repositories []Repository, dryRun bool, recoveryTimeout time.Duration, runner Runner, coordinator Coordinator) (SyncReport, error) {
	report := SyncReport{Operation: OperationSync, DryRun: dryRun, StartedAt: timestamp()}
	if runner == nil {
		runner = CommandRunner{}
	}
	if coordinator == nil {
		coordinator = NoopCoordinator{}
	}
	if recoveryTimeout <= 0 {
		recoveryTimeout = 2 * time.Minute
	}
	for _, repository := range repositories {
		result, failures := syncRepository(ctx, repository, dryRun, recoveryTimeout, runner, coordinator)
		report.Repositories = append(report.Repositories, result)
		report.Failures = append(report.Failures, failures...)
	}
	report.FinishedAt = timestamp()
	if len(report.Failures) != 0 {
		return report, errs.New(errs.ExitPartial, "RG1601", fmt.Sprintf("repository sync completed with %d failure(s)", len(report.Failures)))
	}
	return report, nil
}

func syncRepository(ctx context.Context, repository Repository, dryRun bool, recoveryTimeout time.Duration, runner Runner, coordinator Coordinator) (SyncRepository, []Failure) {
	result := SyncRepository{Name: repository.Name, Aliases: repository.Aliases, Remote: repository.Remote, State: "unavailable", Action: "preserved"}
	branch, remoteOID, err := liveDefault(ctx, runner, repository)
	if err != nil {
		return result, []Failure{{Repository: repository.Name, Operation: "discover-default", Error: err.Error()}}
	}
	if !dryRun {
		if _, err := git(ctx, runner, repository.TopLevel, "fetch", "--prune", repository.Remote); err != nil {
			return result, []Failure{{Repository: repository.Name, Operation: "fetch", Error: err.Error()}}
		}
		branch, remoteOID, err = liveDefault(ctx, runner, repository)
		if err != nil {
			return result, []Failure{{Repository: repository.Name, Operation: "rediscover-default", Error: err.Error()}}
		}
	}
	state := inspectDefault(ctx, runner, repository, branch, remoteOID)
	result.DefaultBranch, result.LocalOID, result.RemoteOID = state.Branch, state.LocalOID, state.RemoteOID
	result.Path, result.State, result.Detail = state.Path, state.State, state.Detail
	if state.Path != "" {
		services, serviceErr := coordinator.AffectedServices(ctx, state.Path)
		if serviceErr != nil {
			return result, []Failure{{Repository: repository.Name, Operation: "inspect-services", Path: state.Path, Error: serviceErr.Error()}}
		}
		result.Services = services
	}
	if state.State != "behind" {
		if dryRun && state.State == "remote-object-unavailable" {
			result.Action = "would-fetch"
		} else if state.State == "current" {
			result.Action = "none"
		}
		return result, nil
	}
	if state.Path != "" {
		clean, cleanErr := cleanWorktree(ctx, runner, state.Path)
		if cleanErr != nil {
			return result, []Failure{{Repository: repository.Name, Operation: "inspect-default-worktree", Path: state.Path, Error: cleanErr.Error()}}
		}
		if !clean {
			result.State, result.Detail = "dirty", "checked-out default branch has local changes"
			return result, nil
		}
	}
	if dryRun {
		result.Action = "would-fast-forward"
		return result, nil
	}
	services, resume, pauseErr := coordinator.Pause(ctx, state.Path)
	if pauseErr != nil {
		result.State, result.Detail = "blocked", "running services could not be paused"
		return result, []Failure{{Repository: repository.Name, Operation: "pause-services", Path: state.Path, Error: pauseErr.Error()}}
	}
	result.Services = services
	updateErr := fastForward(ctx, runner, repository, state)
	resumeContext, cancelResume := context.WithTimeout(context.Background(), recoveryTimeout)
	resumeErr := resume(resumeContext)
	cancelResume()
	if updateErr != nil {
		var failures []Failure
		failures = append(failures, Failure{Repository: repository.Name, Operation: "fast-forward", Path: state.Path, Error: updateErr.Error()})
		if resumeErr != nil {
			failures = append(failures, Failure{Repository: repository.Name, Operation: "resume-services", Path: state.Path, Error: resumeErr.Error()})
		}
		result.Action = "failed"
		var details []string
		if updateErr != nil {
			details = append(details, updateErr.Error())
		}
		if resumeErr != nil {
			details = append(details, resumeErr.Error())
		}
		result.Detail = strings.Join(details, "; ")
		return result, failures
	}
	result.Action, result.State, result.LocalOID = "fast-forwarded", "current", state.RemoteOID
	if resumeErr != nil {
		result.Detail = "default branch advanced but running services could not be resumed: " + resumeErr.Error()
		return result, []Failure{{Repository: repository.Name, Operation: "resume-services", Path: state.Path, Error: resumeErr.Error()}}
	}
	return result, nil
}

func fastForward(ctx context.Context, runner Runner, repository Repository, state defaultState) error {
	if state.Path != "" {
		branch, err := gitText(ctx, runner, state.Path, "symbolic-ref", "--quiet", "--short", "HEAD")
		if err != nil || branch != state.Branch {
			return fmt.Errorf("checked-out default branch changed before fast-forward")
		}
		currentOID, err := gitText(ctx, runner, state.Path, "rev-parse", "HEAD")
		if err != nil || currentOID != state.LocalOID {
			return fmt.Errorf("default branch OID changed before fast-forward")
		}
		clean, err := cleanWorktree(ctx, runner, state.Path)
		if err != nil || !clean {
			return fmt.Errorf("default worktree changed before fast-forward")
		}
		if _, err := git(ctx, runner, state.Path, "merge", "--ff-only", state.RemoteOID); err != nil {
			return err
		}
		updatedOID, err := gitText(ctx, runner, state.Path, "rev-parse", "HEAD")
		if err != nil || updatedOID != state.RemoteOID {
			return fmt.Errorf("default branch did not reach expected remote OID")
		}
		return nil
	}
	_, err := git(ctx, runner, repository.TopLevel, "update-ref", "refs/heads/"+state.Branch, state.RemoteOID, state.LocalOID)
	return err
}
