//go:build darwin || linux

package lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/session"

	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/update"
)

// UpdateCoordinator resolves each service's effective checkout, including
// overrides and worktree selections, and pauses exact running services
// through the same Process Compose and tab-session path that sync uses.
// Without an active runtime it resolves services and reports none running.
type UpdateCoordinator struct {
	loaded *manifest.Loaded
	layout state.Layout
	active *Active
}

func NewUpdateCoordinator(loaded *manifest.Loaded, layout state.Layout, active *Active) *UpdateCoordinator {
	return &UpdateCoordinator{loaded: loaded, layout: layout, active: active}
}

func (c *UpdateCoordinator) Services(ctx context.Context, worktreePath string) ([]update.Service, error) {
	generationID := ""
	if c.active != nil {
		generationID = c.active.Runtime.GenerationID
	}
	var result []update.Service
	for index := range c.loaded.Manifest.Services {
		service := &c.loaded.Manifest.Services[index]
		if service.Source == "external" {
			continue
		}
		execution, err := override.Resolve(ctx, c.layout, generationID, c.loaded, service, nil)
		if err != nil {
			return nil, err
		}
		if !withinMaintenanceWorktree(worktreePath, execution.WorkingDirectory) {
			continue
		}
		item := update.Service{Config: service, WorkingDirectory: execution.WorkingDirectory}
		if c.active != nil {
			current, err := supervisor.Client(c.active.Layout, c.active.Runtime).Get(ctx, service.Name)
			if err != nil {
				return nil, fmt.Errorf("inspect service %s: %w", service.Name, err)
			}
			item.Running = shouldStop(current.Status)
			item.RestartPath = c.restartPath(service)
		}
		result = append(result, item)
	}
	return result, nil
}

// restartPath selects the cooperative session pause only when a tab service
// has a live owning session. A tab service whose session ended, for example
// after its terminal closed, keeps running unowned; it restarts through
// Process Compose like a workspace service, which preserves that state.
func (c *UpdateCoordinator) restartPath(service *manifest.Service) string {
	if service.Activation == "tab" {
		if _, live := session.Active(c.active.Layout, c.active.Runtime.GenerationID, service.Name); live {
			return update.RestartSession
		}
	}
	return update.RestartProcess
}

func (c *UpdateCoordinator) Pause(ctx context.Context, name string) (maintenance.ResumeFunc, error) {
	if c.active == nil {
		return func(context.Context) error { return nil }, nil
	}
	service, exists := manifest.FindService(c.active.Manifest, name)
	if !exists {
		return nil, fmt.Errorf("unknown service %s", name)
	}
	if c.restartPath(service) == update.RestartSession {
		handle, err := session.Pause(ctx, c.active.Layout, c.active.Runtime.GenerationID, name, c.active.Manifest.Runtime.ShutdownTimeout.Duration)
		if err != nil {
			return nil, err
		}
		return handle.Resume, nil
	}
	client := supervisor.Client(c.active.Layout, c.active.Runtime)
	if err := stopForOverride(ctx, *c.active, client, name); err != nil {
		return nil, err
	}
	return func(resumeContext context.Context) error {
		if err := client.Start(resumeContext, name); err != nil {
			return err
		}
		return waitForReadyProcess(resumeContext, client, service, c.active.Manifest.Runtime.StartupTimeout.Duration)
	}, nil
}

// waitForReadyProcess waits for the process itself to run and, when it has a
// health check, to report ready. Unlike waitForService it never accepts a live
// tab shell as readiness, because the shell does not own this restart.
func waitForReadyProcess(ctx context.Context, client processcompose.Client, service *manifest.Service, timeout time.Duration) error {
	waitContext, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		if current, err := client.Get(waitContext, service.Name); err == nil && readyProcessState(service, current) {
			return nil
		}
		select {
		case <-waitContext.Done():
			return errs.Wrap(errs.ExitNotReady, "RG1119", "service did not report ready state: "+service.Name, waitContext.Err())
		case <-ticker.C:
		}
	}
}

var _ update.Coordinator = (*UpdateCoordinator)(nil)
