//go:build darwin || linux

package lifecycle

import (
	"context"
	"fmt"

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
	loaded      *manifest.Loaded
	layout      state.Layout
	active      *Active
	maintenance *MaintenanceCoordinator
}

func NewUpdateCoordinator(loaded *manifest.Loaded, layout state.Layout, active *Active) *UpdateCoordinator {
	coordinator := &UpdateCoordinator{loaded: loaded, layout: layout, active: active}
	if active != nil {
		coordinator.maintenance = NewMaintenanceCoordinator(*active)
	}
	return coordinator
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
		running := false
		if c.active != nil {
			current, err := supervisor.Client(c.active.Layout, c.active.Runtime).Get(ctx, service.Name)
			if err != nil {
				return nil, fmt.Errorf("inspect service %s: %w", service.Name, err)
			}
			running = shouldStop(current.Status)
		}
		result = append(result, update.Service{Config: service, WorkingDirectory: execution.WorkingDirectory, Running: running})
	}
	return result, nil
}

func (c *UpdateCoordinator) Pause(ctx context.Context, names []string) (maintenance.ResumeFunc, error) {
	if c.maintenance == nil || len(names) == 0 {
		return func(context.Context) error { return nil }, nil
	}
	selected := map[string]bool{}
	for _, name := range names {
		selected[name] = true
	}
	var services []*manifest.Service
	for index := range c.active.Manifest.Services {
		if service := &c.active.Manifest.Services[index]; selected[service.Name] {
			services = append(services, service)
		}
	}
	return c.maintenance.pauseServices(ctx, services)
}

var _ update.Coordinator = (*UpdateCoordinator)(nil)
