package update

import (
	"context"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

// apply pauses exactly the services that need a restart, advances the default
// branch, runs changed units in order, and always resumes what it paused.
// Fingerprints are recorded only for units and restarts that succeeded, so a
// failure is retried by the next run and a clean run is a no-op.
func (e *engine) apply(ctx context.Context, repository maintenance.Repository, checkout maintenance.DefaultCheckout, plan repositoryPlan, advance bool, result RepositoryResult) (RepositoryResult, []maintenance.Failure) {
	resume, err := e.deps.Coordinator.Pause(ctx, plan.restartNames())
	if err != nil {
		result.Status, result.Reason = StatusFailed, "pause-services: "+err.Error()
		for _, item := range plan.units {
			item.result.Status, item.result.Reason = StatusSkipped, "running services could not be paused"
			result.Steps = append(result.Steps, item.result)
		}
		for _, service := range plan.services {
			if service.restart {
				service.result.Status, service.result.Reason = StatusFailed, "could not be paused: "+err.Error()
			}
			result.Services = append(result.Services, service.result)
		}
		return result, []maintenance.Failure{{Repository: repository.Name, Operation: "pause-services", Path: checkout.Path, Error: err.Error()}}
	}
	var failures []maintenance.Failure
	recorded := e.inputs.Repositories[checkout.Path]
	inputs := map[string]string{}
	for key, value := range recorded.Inputs {
		inputs[key] = value
	}
	changedAnything, blocked, advanced := false, "", true
	if advance {
		if err := maintenance.FastForward(ctx, e.deps.Runner, repository, checkout); err != nil {
			failures = append(failures, maintenance.Failure{Repository: repository.Name, Operation: "fast-forward", Path: checkout.Path, Error: err.Error()})
			result.Git, blocked, advanced = "failed", "fast-forward failed", false
		} else {
			result.Git, changedAnything = "fast-forwarded", true
		}
	}
	for _, item := range plan.units {
		switch {
		case !item.run:
			item.result.Status = StatusCurrent
			inputs[item.key] = item.fingerprint
		case blocked != "":
			item.result.Status, item.result.Reason = StatusSkipped, blocked
		default:
			log, runErr := e.deps.Executor.Execute(ctx, e.commandIndex, item.command)
			e.commandIndex++
			item.result.Log = log
			if runErr != nil {
				item.result.Status, item.result.Reason = StatusFailed, runErr.Error()
				failures = append(failures, maintenance.Failure{Repository: repository.Name, Operation: item.result.ID, Path: checkout.Path, Error: runErr.Error()})
				blocked = item.result.Name + " failed"
			} else {
				item.result.Status = StatusUpdated
				inputs[item.key], changedAnything = item.fingerprint, true
			}
		}
		result.Steps = append(result.Steps, item.result)
	}
	resumeContext, cancel := context.WithTimeout(context.Background(), e.recoveryTimeout)
	resumeErr := resume(resumeContext)
	cancel()
	if resumeErr != nil {
		failures = append(failures, maintenance.Failure{Repository: repository.Name, Operation: "resume-services", Path: checkout.Path, Error: resumeErr.Error()})
	}
	for _, service := range plan.services {
		switch {
		case service.restart && resumeErr != nil:
			service.result.Status, service.result.Reason = StatusFailed, "restart failed: "+resumeErr.Error()
		default:
			inputs[service.watchKey] = service.watchPrint
			changedAnything = changedAnything || service.restart
		}
		result.Services = append(result.Services, service.result)
	}
	if advanced {
		e.inputs.Repositories[checkout.Path] = repositoryInput{OID: result.ToOID, Inputs: inputs, UpdatedAt: timestamp()}
	}
	switch {
	case len(failures) != 0:
		result.Status, result.Reason = StatusFailed, failures[0].Operation+": "+failures[0].Error
	case changedAnything:
		result.Status = StatusUpdated
	default:
		result.Status = StatusCurrent
	}
	return result, failures
}
