package update

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

// Dependencies are the side-effecting collaborators of one update run.
type Dependencies struct {
	Layout      state.Layout
	Runner      maintenance.Runner
	Coordinator Coordinator
	Executor    Executor
}

type engine struct {
	options         Options
	deps            Dependencies
	recoveryTimeout time.Duration
	inputs          store
	commandIndex    int
}

// Run updates every selected repository. One repository's failure never
// stops another, and every paused service is resumed before Run returns.
func Run(ctx context.Context, loaded *manifest.Loaded, options Options, deps Dependencies) (Report, error) {
	if deps.Runner == nil {
		deps.Runner = maintenance.CommandRunner{}
	}
	repositories, failures := maintenance.Discover(ctx, loaded, options.Repositories, deps.Runner)
	timeout := loaded.Manifest.Runtime.StartupTimeout.Duration + loaded.Manifest.Runtime.ShutdownTimeout.Duration
	return runRepositories(ctx, repositories, failures, timeout, options, deps)
}

func runRepositories(ctx context.Context, repositories []maintenance.Repository, failures []maintenance.Failure, recoveryTimeout time.Duration, options Options, deps Dependencies) (Report, error) {
	report := Report{Operation: Operation, DryRun: options.DryRun, Baseline: options.Baseline, StartedAt: timestamp(), Failures: failures}
	if report.Failures == nil {
		report.Failures = []maintenance.Failure{}
	}
	if deps.Runner == nil {
		deps.Runner = maintenance.CommandRunner{}
	}
	if recoveryTimeout <= 0 {
		recoveryTimeout = 2 * time.Minute
	}
	if !options.DryRun {
		if err := deps.Layout.Ensure(); err != nil {
			report.FinishedAt = timestamp()
			return report, err
		}
	}
	inputs, err := loadStore(deps.Layout)
	if err != nil {
		report.FinishedAt = timestamp()
		return report, err
	}
	run := &engine{options: options, deps: deps, recoveryTimeout: recoveryTimeout, inputs: inputs}
	for _, repository := range repositories {
		result, repositoryFailures := run.repository(ctx, repository)
		report.Repositories = append(report.Repositories, result)
		report.Failures = append(report.Failures, repositoryFailures...)
		if options.DryRun {
			continue
		}
		if err := saveStore(deps.Layout, run.inputs); err != nil {
			report.Failures = append(report.Failures, maintenance.Failure{Repository: repository.Name, Operation: "record-inputs", Error: err.Error()})
		}
	}
	report.FinishedAt = timestamp()
	if len(report.Failures) != 0 {
		return report, errs.New(errs.ExitPartial, "RG1903", fmt.Sprintf("workspace update completed with %d failure(s)", len(report.Failures)))
	}
	return report, nil
}

func (e *engine) repository(ctx context.Context, repository maintenance.Repository) (RepositoryResult, []maintenance.Failure) {
	result := RepositoryResult{Name: repository.Name, Status: StatusSkipped, Git: "preserved", Steps: []StepResult{}, Services: []ServiceResult{}}
	fail := func(operation, path string, err error) (RepositoryResult, []maintenance.Failure) {
		result.Status, result.Reason = StatusFailed, operation+": "+err.Error()
		return result, []maintenance.Failure{{Repository: repository.Name, Operation: operation, Path: path, Error: err.Error()}}
	}
	fetch := !e.options.DryRun && !e.options.Baseline
	checkout, failure := maintenance.InspectDefault(ctx, e.deps.Runner, repository, fetch)
	if failure != nil {
		result.Status, result.Reason = StatusFailed, failure.Operation+": "+failure.Error
		return result, []maintenance.Failure{*failure}
	}
	result.Path, result.DefaultBranch, result.FromOID, result.ToOID = checkout.Path, checkout.Branch, checkout.LocalOID, checkout.RemoteOID
	advance := checkout.State == "behind" && !e.options.Baseline
	switch {
	case checkout.State == "current", checkout.State == "behind":
	case checkout.State == "remote-object-unavailable" && e.options.DryRun:
		result.Status, result.Git, result.Reason = StatusUpdated, "would-fetch", "remote commits are not fetched; the step plan is available after fetch"
		return result, nil
	default:
		result.Reason = checkout.State
		if checkout.Detail != "" {
			result.Reason += ": " + checkout.Detail
		}
		return result, nil
	}
	if !advance {
		result.Git, result.ToOID = "none", checkout.LocalOID
	}
	if checkout.Path == "" {
		return e.unattached(ctx, repository, checkout, result, advance, fail)
	}
	clean, err := maintenance.CleanWorktree(ctx, e.deps.Runner, checkout.Path)
	if err != nil {
		return fail("inspect-default-worktree", checkout.Path, err)
	}
	if !clean {
		result.Reason = "dirty: checked-out default branch has local changes"
		return result, nil
	}
	target, err := listTree(ctx, e.deps.Runner, checkout.Path, result.ToOID)
	if err != nil {
		return fail("inspect-inputs", checkout.Path, err)
	}
	var previous tree
	if advance {
		if previous, err = listTree(ctx, e.deps.Runner, checkout.Path, checkout.LocalOID); err != nil {
			return fail("inspect-inputs", checkout.Path, err)
		}
	}
	services, err := e.deps.Coordinator.Services(ctx, checkout.Path)
	if err != nil {
		return fail("inspect-services", checkout.Path, err)
	}
	recorded := e.inputs.Repositories[checkout.Path]
	plan := buildPlan(checkout.Path, services, target, previous, recorded.Inputs)
	switch {
	case e.options.Baseline:
		e.recordBaseline(checkout.Path, result.ToOID, plan, &result)
		return result, nil
	case e.options.DryRun:
		previewPlan(plan, advance, &result)
		return result, nil
	}
	return e.apply(ctx, repository, checkout, plan, advance, result)
}

// unattached handles a default branch that no worktree checks out: only the
// ref advances, and no dependency, step, or service is involved.
func (e *engine) unattached(ctx context.Context, repository maintenance.Repository, checkout maintenance.DefaultCheckout, result RepositoryResult, advance bool, fail func(string, string, error) (RepositoryResult, []maintenance.Failure)) (RepositoryResult, []maintenance.Failure) {
	result.Status, result.Reason = StatusCurrent, "default branch is not checked out; no services or dependencies"
	if !advance {
		return result, nil
	}
	if e.options.DryRun {
		result.Status, result.Git = StatusUpdated, "would-fast-forward"
		return result, nil
	}
	if err := maintenance.FastForward(ctx, e.deps.Runner, repository, checkout); err != nil {
		return fail("fast-forward", "", err)
	}
	result.Status, result.Git = StatusUpdated, "fast-forwarded"
	return result, nil
}

func (e *engine) recordBaseline(path, oid string, plan repositoryPlan, result *RepositoryResult) {
	inputs := map[string]string{}
	for _, item := range plan.units {
		inputs[item.key] = item.fingerprint
		item.result.Status, item.result.Reason = StatusCurrent, "baseline recorded"
		result.Steps = append(result.Steps, item.result)
	}
	for _, service := range plan.services {
		inputs[service.watchKey] = service.watchPrint
		result.Services = append(result.Services, ServiceResult{Name: service.name, Restart: service.policy, Status: StatusCurrent, Reason: "baseline recorded"})
	}
	e.inputs.Repositories[path] = repositoryInput{OID: oid, Inputs: inputs, UpdatedAt: timestamp()}
	result.Status, result.Git, result.Reason = StatusCurrent, "not-fetched", "current inputs recorded as up to date"
}

func previewPlan(plan repositoryPlan, advance bool, result *RepositoryResult) {
	result.Status = StatusCurrent
	if advance {
		result.Status, result.Git = StatusUpdated, "would-fast-forward"
	}
	for _, item := range plan.units {
		item.result.Status = StatusCurrent
		if item.run {
			item.result.Status, result.Status = StatusUpdated, StatusUpdated
		}
		result.Steps = append(result.Steps, item.result)
	}
	for _, service := range plan.services {
		if service.restart {
			result.Status = StatusUpdated
		}
		result.Services = append(result.Services, service.result)
	}
}
