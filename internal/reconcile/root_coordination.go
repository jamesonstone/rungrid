package reconcile

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

type pausedRoot struct {
	state    rootState
	services []string
	owned    []string
	resume   maintenance.ResumeFunc
}

func requireRootProcessProof(ctx context.Context, state rootState, result *RootResult, options Options) error {
	services, owned, err := rootOwnership(ctx, state.path, options.Coordinator)
	result.Services = services
	result.OwnedProcessIDs = owned
	if err != nil {
		result.Reason, result.Detail = "process-ownership-failed", err.Error()
		return err
	}
	if state.processErr != nil {
		result.Reason, result.Detail = "process-inspection-failed", state.processErr.Error()
		return state.processErr
	}
	if len(withoutStrings(state.processes, owned)) != 0 {
		result.Reason = "primary-in-use"
	}
	return nil
}

func pauseRoot(ctx context.Context, repository maintenance.Repository, expected rootState, options Options) (pausedRoot, error) {
	services, owned, err := rootOwnership(ctx, expected.path, options.Coordinator)
	if err != nil {
		return pausedRoot{}, err
	}
	pausedServices, resume, err := options.Coordinator.Pause(ctx, expected.path)
	if err != nil {
		return pausedRoot{}, err
	}
	if len(pausedServices) != 0 {
		services = pausedServices
	}
	fresh, inspectErr := inspectRoot(ctx, repository, options.Runner)
	if inspectErr == nil {
		_, currentOwned, ownerErr := rootOwnership(ctx, expected.path, options.Coordinator)
		owned = uniqueStrings(owned, currentOwned)
		inspectErr = ownerErr
	}
	if inspectErr == nil && !sameRootState(fresh, expected) {
		inspectErr = fmt.Errorf("primary root changed before recovery")
	}
	if inspectErr == nil && (fresh.processErr != nil || len(withoutStrings(fresh.processes, owned)) != 0) {
		inspectErr = fmt.Errorf("unowned primary-root process activity remains after service pause")
	}
	if inspectErr != nil {
		return pausedRoot{}, errors.Join(inspectErr, resumeWithTimeout(resume, options.RecoveryTimeout))
	}
	return pausedRoot{state: fresh, services: services, owned: owned, resume: resume}, nil
}

func rootOwnership(ctx context.Context, path string, coordinator maintenance.Coordinator) ([]string, []string, error) {
	services, err := coordinator.AffectedServices(ctx, path)
	if err != nil {
		return nil, nil, err
	}
	owner, ok := coordinator.(maintenance.ProcessOwner)
	if !ok {
		return services, nil, nil
	}
	owned, err := owner.OwnedProcessIDs(ctx, path)
	return services, owned, err
}

func sameRootState(current, expected rootState) bool {
	return current.branch == expected.branch && current.headOID == expected.headOID &&
		current.staged == expected.staged && current.conflicted == expected.conflicted &&
		reflect.DeepEqual(current.dirtyPaths, expected.dirtyPaths)
}

func finishRootMutation(paused pausedRoot, timeout time.Duration, mutationErr error) error {
	return errors.Join(mutationErr, resumeWithTimeout(paused.resume, timeout))
}

func resumeWithTimeout(resume maintenance.ResumeFunc, timeout time.Duration) error {
	if resume == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return resume(ctx)
}
