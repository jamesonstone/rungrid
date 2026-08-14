//go:build darwin || linux

package lifecycle

import (
	"context"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
)

func waitForWorkspaceReady(ctx context.Context, client processcompose.Client, configuration *manifest.Manifest) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		states, _, err := client.List(ctx)
		if err == nil && workspaceServicesReady(configuration, states) {
			return nil
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(errs.ExitNotReady, "RG1105", "workspace-owned services did not become ready", ctx.Err())
		case <-ticker.C:
		}
	}
}

func workspaceServicesReady(configuration *manifest.Manifest, states []processcompose.ProcessState) bool {
	byName := make(map[string]processcompose.ProcessState, len(states))
	for _, current := range states {
		byName[current.Name] = current
	}
	guard, exists := byName["rungrid-resource-guard"]
	if !exists || !readyProcessState(&manifest.Service{}, guard) {
		return false
	}
	for index := range configuration.Services {
		service := &configuration.Services[index]
		if service.Source == "external" || service.Activation != "workspace" {
			continue
		}
		current, exists := byName[service.Name]
		if !exists || !readyProcessState(service, current) {
			return false
		}
	}
	return true
}

func readyProcessState(service *manifest.Service, current processcompose.ProcessState) bool {
	status := strings.ToLower(current.Status)
	if !strings.Contains(status, "running") && !strings.Contains(status, "launched") {
		return false
	}
	if service.Health == nil {
		return true
	}
	health := strings.ToLower(current.Health)
	return strings.Contains(health, "ready") || strings.Contains(health, "healthy")
}
