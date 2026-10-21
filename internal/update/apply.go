package update

import (
	"context"
	"strings"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

// apply pauses exactly the services that need a restart, advances the default
// branch, runs changed units in order, and always resumes what it paused.
// Fingerprints are recorded only for units and restarts that succeeded, so a
// failure is retried by the next run and a clean run is a no-op.
func (e *engine) apply(ctx context.Context, repository maintenance.Repository, checkout maintenance.DefaultCheckout, plan repositoryPlan, advance bool, result RepositoryResult) (RepositoryResult, []maintenance.Failure) {
	paused, failure := e.pauseAll(ctx, plan.restartNames())
	if failure != nil {
		return e.pauseFailed(repository, checkout, plan, result, *failure)
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
	resumeErrs := e.resumeAll(paused)
	for _, service := range plan.services {
		resumeErr := resumeErrs[service.name]
		switch {
		case service.restart && resumeErr != nil:
			service.result.Status, service.result.Reason = StatusFailed, "restart failed: "+resumeErr.Error()
			failures = append(failures, maintenance.Failure{Repository: repository.Name, Operation: "restart " + service.name, Path: checkout.Path, Error: resumeErr.Error()})
		default:
			inputs[service.watchKey] = service.watchPrint
			changedAnything = changedAnything || service.restart
			if detail, ok := e.recovered[service.name]; ok && service.restart {
				service.result.RestartPath = RestartProcess
				service.result.Reason = strings.TrimSuffix(service.result.Reason, "; restart through its tab session") + "; " + detail
			}
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

// pauseFailed reports a repository whose services could not all be paused.
// Nothing changed: no ref moved, no unit ran, and every service that was
// paused has been restored.
func (e *engine) pauseFailed(repository maintenance.Repository, checkout maintenance.DefaultCheckout, plan repositoryPlan, result RepositoryResult, failure pauseFailure) (RepositoryResult, []maintenance.Failure) {
	failedName, pauseErr := failure.service, failure.err
	failures := []maintenance.Failure{{Repository: repository.Name, Operation: "pause " + failedName, Path: checkout.Path, Error: pauseErr.Error()}}
	result.Status, result.Reason = StatusFailed, "pause "+failedName+": "+pauseErr.Error()+"; nothing was changed"
	for _, item := range plan.units {
		if item.run {
			item.result.Status, item.result.Reason = StatusSkipped, "not run: "+failedName+" could not be paused"
		} else {
			item.result.Status = StatusCurrent
		}
		result.Steps = append(result.Steps, item.result)
	}
	for _, service := range plan.services {
		restoreErr := failure.restored[service.name]
		switch {
		case service.name == failedName:
			service.result.Status, service.result.Reason = StatusFailed, "could not be paused: "+pauseErr.Error()
		case restoreErr == nil && e.recovered[service.name] != "":
			service.result.Status, service.result.Reason = StatusSkipped, "not restarted: "+failedName+" could not be paused; "+e.recovered[service.name]
		case restoreErr != nil:
			service.result.Status, service.result.Reason = StatusFailed, "restore after pause failure failed: "+restoreErr.Error()
			failures = append(failures, maintenance.Failure{Repository: repository.Name, Operation: "restore " + service.name, Path: checkout.Path, Error: restoreErr.Error()})
		case service.restart:
			service.result.Status, service.result.Reason = StatusSkipped, "not restarted: "+failedName+" could not be paused; left running as before"
		}
		result.Services = append(result.Services, service.result)
	}
	return result, failures
}
