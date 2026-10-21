//go:build darwin || linux

package session

import (
	"context"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
)

// startPausedProcess starts the paused process. A process that is already
// running, for example after a maintenance caller restarted it through Process
// Compose, is adopted rather than treated as a failure; a process still
// terminating is waited out first.
func startPausedProcess(ctx context.Context, options Options, client processcompose.Client) error {
	if current, err := client.Get(ctx, options.Service); err == nil && isRunning(current.Status) {
		return nil
	}
	if err := processcompose.WaitSettled(ctx, client, options.Service, options.Manifest.Runtime.ShutdownTimeout.Duration); err != nil {
		return err
	}
	return client.Start(ctx, options.Service)
}

func waitForResumedService(ctx context.Context, options Options, client processcompose.Client, service *manifest.Service) error {
	waitContext, cancel := context.WithTimeout(ctx, options.Manifest.Runtime.StartupTimeout.Duration)
	defer cancel()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		current, err := client.Get(waitContext, options.Service)
		if err == nil {
			status := strings.ToLower(current.Status)
			healthy := service.Health == nil || processcompose.HealthReady(current.Health)
			if healthy && (strings.Contains(status, "running") || strings.Contains(status, "healthy")) {
				return nil
			}
		}
		select {
		case <-waitContext.Done():
			return errs.Wrap(errs.ExitNotReady, "RG823", "resumed service did not report ready state: "+options.Service, waitContext.Err())
		case <-ticker.C:
		}
	}
}
