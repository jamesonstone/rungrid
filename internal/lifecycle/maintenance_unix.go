//go:build darwin || linux

package lifecycle

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/session"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

type MaintenanceCoordinator struct {
	active Active
}

type pausedMaintenanceService struct {
	service *manifest.Service
	handle  *session.MaintenanceHandle
}

func NewMaintenanceCoordinator(active Active) *MaintenanceCoordinator {
	return &MaintenanceCoordinator{active: active}
}

func (c *MaintenanceCoordinator) AffectedServices(ctx context.Context, worktreePath string) ([]string, error) {
	services, err := c.runningServices(ctx, worktreePath)
	if err != nil {
		return nil, err
	}
	result := make([]string, len(services))
	for index, service := range services {
		result[index] = service.Name
	}
	return result, nil
}

func (c *MaintenanceCoordinator) OwnedProcessIDs(ctx context.Context, worktreePath string) ([]string, error) {
	services, err := c.runningServices(ctx, worktreePath)
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool)
	if c.active.Runtime.PID > 0 && withinMaintenanceWorktree(worktreePath, c.active.Runtime.WorkspaceRoot) {
		seen[c.active.Runtime.PID] = true
	}
	client := supervisor.Client(c.active.Layout, c.active.Runtime)
	for _, service := range services {
		current, getErr := client.Get(ctx, service.Name)
		if getErr != nil {
			return nil, fmt.Errorf("inspect owned service %s: %w", service.Name, getErr)
		}
		if current.PID > 0 {
			seen[current.PID] = true
		}
		if service.Activation == "tab" {
			registration, live := session.Active(c.active.Layout, c.active.Runtime.GenerationID, service.Name)
			if !live {
				return nil, fmt.Errorf("tab-owned service %s has no verified owning session", service.Name)
			}
			seen[registration.PID] = true
		}
	}
	result := make([]string, 0, len(seen))
	for pid := range seen {
		result = append(result, strconv.Itoa(pid))
	}
	sort.Strings(result)
	return result, nil
}

func (c *MaintenanceCoordinator) Pause(ctx context.Context, worktreePath string) ([]string, maintenance.ResumeFunc, error) {
	services, err := c.runningServices(ctx, worktreePath)
	if err != nil || len(services) == 0 {
		return nil, func(context.Context) error { return nil }, err
	}
	paused := make([]pausedMaintenanceService, 0, len(services))
	client := supervisor.Client(c.active.Layout, c.active.Runtime)
	for index := len(services) - 1; index >= 0; index-- {
		service := services[index]
		item := pausedMaintenanceService{service: service}
		if service.Activation == "tab" {
			timeout := c.active.Manifest.Runtime.ShutdownTimeout.Duration
			handle, pauseErr := session.Pause(ctx, c.active.Layout, c.active.Runtime.GenerationID, service.Name, timeout)
			if pauseErr != nil {
				c.resumePausedAfterFailure(paused)
				return serviceNames(services), nil, pauseErr
			}
			item.handle = handle
		} else if stopErr := client.Stop(ctx, service.Name); stopErr != nil {
			c.resumePausedAfterFailure(paused)
			return serviceNames(services), nil, stopErr
		}
		paused = append([]pausedMaintenanceService{item}, paused...)
	}
	return serviceNames(services), func(resumeContext context.Context) error {
		return c.resume(resumeContext, paused)
	}, nil
}

func (c *MaintenanceCoordinator) resumePausedAfterFailure(paused []pausedMaintenanceService) {
	timeout := c.active.Manifest.Runtime.StartupTimeout.Duration + c.active.Manifest.Runtime.ShutdownTimeout.Duration
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	_ = c.resume(ctx, paused)
}

func (c *MaintenanceCoordinator) resume(ctx context.Context, paused []pausedMaintenanceService) error {
	client := supervisor.Client(c.active.Layout, c.active.Runtime)
	for _, item := range paused {
		if item.handle != nil {
			if err := item.handle.Resume(ctx); err != nil {
				return err
			}
			continue
		}
		if err := client.Start(ctx, item.service.Name); err != nil {
			return err
		}
		waitContext, cancel := context.WithTimeout(ctx, c.active.Manifest.Runtime.StartupTimeout.Duration)
		err := waitForService(waitContext, client, c.active.Layout, c.active.Runtime.GenerationID, item.service)
		cancel()
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *MaintenanceCoordinator) runningServices(ctx context.Context, worktreePath string) ([]*manifest.Service, error) {
	if worktreePath == "" {
		return nil, nil
	}
	worktreePath = filepath.Clean(worktreePath)
	client := supervisor.Client(c.active.Layout, c.active.Runtime)
	var result []*manifest.Service
	for index := range c.active.Manifest.Services {
		service := &c.active.Manifest.Services[index]
		if service.Source == "external" {
			continue
		}
		repositoryRoot, err := manifest.ServiceRepositoryRoot(c.active.Manifest, c.active.Runtime.WorkspaceRoot, service)
		if err != nil {
			return nil, err
		}
		if !withinMaintenanceWorktree(worktreePath, repositoryRoot) {
			continue
		}
		current, err := client.Get(ctx, service.Name)
		if err != nil {
			return nil, fmt.Errorf("inspect service %s: %w", service.Name, err)
		}
		if shouldStop(current.Status) {
			result = append(result, service)
		}
	}
	return result, nil
}

func withinMaintenanceWorktree(root, candidate string) bool {
	relative, err := filepath.Rel(filepath.Clean(root), filepath.Clean(candidate))
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func serviceNames(services []*manifest.Service) []string {
	result := make([]string, len(services))
	for index, service := range services {
		result[index] = service.Name
	}
	return result
}

var _ maintenance.Coordinator = (*MaintenanceCoordinator)(nil)
var _ maintenance.ProcessOwner = (*MaintenanceCoordinator)(nil)
