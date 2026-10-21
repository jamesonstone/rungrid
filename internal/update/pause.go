package update

import (
	"context"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

type pausedService struct {
	name   string
	resume maintenance.ResumeFunc
}

// pauseFailure names the service whose pause failed and the outcome of
// restoring the services paused before it.
type pauseFailure struct {
	service  string
	err      error
	restored map[string]error
}

// pauseAll pauses services in reverse manifest order, so dependents stop
// first. If one pause fails, every service already paused is restored before
// pauseAll returns, leaving the workspace as it was.
func (e *engine) pauseAll(ctx context.Context, names []string) ([]pausedService, *pauseFailure) {
	var paused []pausedService
	for index := len(names) - 1; index >= 0; index-- {
		resume, err := e.deps.Coordinator.Pause(ctx, names[index])
		if err != nil {
			return nil, &pauseFailure{service: names[index], err: err, restored: e.resumeAll(paused)}
		}
		paused = append([]pausedService{{name: names[index], resume: resume}}, paused...)
	}
	return paused, nil
}

// resumeAll restarts paused services in manifest order. Each service gets its
// own fresh recovery deadline, independent of the caller's context, and one
// failure does not stop the others from restarting.
func (e *engine) resumeAll(paused []pausedService) map[string]error {
	failures := map[string]error{}
	for _, item := range paused {
		ctx, cancel := context.WithTimeout(context.Background(), e.recoveryTimeout)
		if err := item.resume(ctx); err != nil {
			failures[item.name] = err
		}
		cancel()
	}
	return failures
}
