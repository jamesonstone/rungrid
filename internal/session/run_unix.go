//go:build darwin || linux

package session

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/serviceexec"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

type Options struct {
	Layout   state.Layout
	Runtime  supervisor.Runtime
	Manifest *manifest.Manifest
	Service  string
	TabID    string
	Stdin    io.Reader
	Stdout   io.Writer
	Stderr   io.Writer
}

func Run(ctx context.Context, options Options) (returnErr error) {
	service, err := validateSession(ctx, options)
	if err != nil {
		return err
	}
	lock, err := Acquire(options.Layout, options.Runtime.GenerationID, options.Service, options.TabID)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := lock.Release(); returnErr == nil && releaseErr != nil {
			returnErr = releaseErr
		}
	}()
	client := supervisor.Client(options.Layout, options.Runtime)
	if current, getErr := client.Get(ctx, options.Service); getErr == nil && isRunning(current.Status) {
		return errs.New(errs.ExitConflict, "RG810", "tab-owned service is already running without this session")
	}
	if err := client.Start(ctx, options.Service); err != nil {
		return err
	}
	owned := true
	defer func() {
		if owned {
			stopContext, cancel := context.WithTimeout(context.Background(), options.Manifest.Runtime.ShutdownTimeout.Duration)
			_ = client.Stop(stopContext, options.Service)
			cancel()
		}
	}()
	follower, err := startLogFollower(options.Layout, options.Runtime, client, options.Service, options.Stdin, options.Stdout, options.Stderr)
	if err != nil {
		return err
	}
	defer func() { _ = follower.stop() }()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	started, paused, resourcePaused := false, false, false
	lastIncidentID := ""
	var pausedRequest maintenanceRequest
	for {
		select {
		case <-ctx.Done():
			if !paused {
				stopContext, cancel := context.WithTimeout(context.Background(), options.Manifest.Runtime.ShutdownTimeout.Duration)
				_ = client.Stop(stopContext, options.Service)
				cancel()
			}
			owned = false
			_ = follower.stop()
			return errs.Wrap(errs.ExitInterrupted, "RG812", "service session interrupted", ctx.Err())
		case <-follower.channel():
			logErr := follower.err()
			if reason := quiesceReason(options); reason != "" {
				owned = false
				follower = nil
				_, _ = fmt.Fprintf(options.Stdout, "\n[rungrid] %s session released: %s\n", options.Service, reason)
				return nil
			}
			if request, exists := readMaintenanceRequest(options.Layout, lock.identity); exists && request.Action == "pause" {
				follower = nil
				if err := pauseSession(context.Background(), options, client, request); err != nil {
					return err
				}
				paused, pausedRequest = true, request
				continue
			}
			if waiting, incident := resourceRestartWaiting(options); waiting {
				follower = nil
				resourcePaused = true
				lastIncidentID = printResourceIncident(options.Stdout, incident, lastIncidentID)
				continue
			}
			if logErr != nil && !errors.Is(logErr, context.Canceled) {
				return errs.Wrap(errs.ExitFailure, "RG813", "service log foreground ended", logErr)
			}
			return nil
		case <-ticker.C:
			if reason := quiesceReason(options); reason != "" {
				owned = false
				_ = follower.stop()
				_, _ = fmt.Fprintf(options.Stdout, "\n[rungrid] %s session released: %s\n", options.Service, reason)
				return nil
			}
			if waiting, incident := resourceRestartWaiting(options); waiting {
				lastIncidentID = printResourceIncident(options.Stdout, incident, lastIncidentID)
				if !resourcePaused {
					_ = follower.stop()
					follower = nil
					resourcePaused = true
				}
				current, getErr := client.Get(ctx, options.Service)
				if getErr != nil || !isRunning(current.Status) {
					continue
				}
				resumed, followerErr := startLogFollower(options.Layout, options.Runtime, client, options.Service, options.Stdin, options.Stdout, options.Stderr)
				if followerErr != nil {
					return followerErr
				}
				follower, resourcePaused, started = resumed, false, true
				continue
			}
			request, hasRequest := readMaintenanceRequest(options.Layout, lock.identity)
			if hasRequest && request.Action == "pause" && !paused {
				_ = follower.stop()
				follower = nil
				if err := pauseSession(ctx, options, client, request); err != nil {
					return err
				}
				paused, pausedRequest = true, request
				continue
			}
			resumeRequested := hasRequest && request.RequestID == pausedRequest.RequestID && request.Action == "resume"
			expired := requestExpired(pausedRequest)
			if paused && (resumeRequested || expired) {
				resumeRequest := pausedRequest
				if resumeRequested {
					resumeRequest = request
				}
				resumed, resumeErr := resumeSession(ctx, options, client, service, resumeRequest)
				if resumeErr != nil {
					return resumeErr
				}
				follower, paused, started = resumed, false, false
				if expired && !resumeRequested {
					removeMaintenanceTransition(options.Layout, pausedRequest)
				}
				continue
			}
			if paused {
				continue
			}
			if resourcePaused {
				current, getErr := client.Get(ctx, options.Service)
				if getErr != nil || !isRunning(current.Status) {
					continue
				}
				resumed, followerErr := startLogFollower(options.Layout, options.Runtime, client, options.Service, options.Stdin, options.Stdout, options.Stderr)
				if followerErr != nil {
					return followerErr
				}
				follower, resourcePaused, started = resumed, false, true
				continue
			}
			current, getErr := client.Get(ctx, options.Service)
			if getErr != nil {
				continue
			}
			if isRunning(current.Status) {
				started = true
				continue
			}
			if started || isTerminal(current.Status) {
				owned = false
				_ = follower.stop()
				if current.ExitCode != 0 {
					return errs.New(errs.ExitFailure, "RG814", fmt.Sprintf("service %s exited with code %d", options.Service, current.ExitCode))
				}
				return nil
			}
		}
	}
}

func validateSession(ctx context.Context, options Options) (*manifest.Service, error) {
	service, exists := manifest.FindService(options.Manifest, options.Service)
	if !exists {
		return nil, errs.New(errs.ExitUsage, "RG808", "unknown service: "+options.Service)
	}
	if service.Activation != "tab" || service.Source == "external" {
		return nil, errs.New(errs.ExitUsage, "RG809", "session requires a tab-owned native or Compose service")
	}
	if err := supervisor.Verify(ctx, options.Layout, options.Runtime); err != nil {
		return nil, err
	}
	for dependency := range service.DependsOn {
		candidate, _ := manifest.FindService(options.Manifest, dependency)
		if candidate == nil || candidate.Source != "external" {
			continue
		}
		waitContext, cancel := context.WithTimeout(ctx, options.Manifest.Runtime.StartupTimeout.Duration)
		err := serviceexec.WaitExternal(waitContext, options.Manifest, options.Runtime.WorkspaceRoot, candidate)
		cancel()
		if err != nil {
			return nil, err
		}
	}
	return service, nil
}

func pauseSession(ctx context.Context, options Options, client processcompose.Client, request maintenanceRequest) error {
	stopContext, cancel := context.WithTimeout(ctx, options.Manifest.Runtime.ShutdownTimeout.Duration)
	err := client.Stop(stopContext, options.Service)
	cancel()
	ackErr := acknowledgeMaintenance(options.Layout, request, "paused", err)
	if err == nil {
		_, _ = fmt.Fprintf(options.Stdout, "\n[rungrid] %s paused for repository maintenance\n", options.Service)
	}
	return errors.Join(err, ackErr)
}

func resumeSession(ctx context.Context, options Options, client processcompose.Client, service *manifest.Service, request maintenanceRequest) (*logFollower, error) {
	if err := client.Start(ctx, options.Service); err != nil {
		ackErr := acknowledgeMaintenance(options.Layout, request, "failed", err)
		return nil, errors.Join(err, ackErr)
	}
	follower, err := startLogFollower(options.Layout, options.Runtime, client, options.Service, options.Stdin, options.Stdout, options.Stderr)
	if err != nil {
		ackErr := acknowledgeMaintenance(options.Layout, request, "failed", err)
		return nil, errors.Join(err, ackErr)
	}
	readyErr := waitForResumedService(ctx, options, client, service)
	ackErr := acknowledgeMaintenance(options.Layout, request, "running", readyErr)
	if readyErr != nil {
		_ = follower.stop()
		return nil, errors.Join(readyErr, ackErr)
	}
	if ackErr != nil {
		_ = follower.stop()
		return nil, ackErr
	}
	_, _ = fmt.Fprintf(options.Stdout, "[rungrid] %s resumed after repository maintenance\n", options.Service)
	return follower, nil
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
			healthy := service.Health == nil || strings.Contains(strings.ToLower(current.Health), "healthy")
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

func requestExpired(request maintenanceRequest) bool {
	expiresAt, err := time.Parse(time.RFC3339Nano, request.ExpiresAt)
	return err != nil || time.Now().After(expiresAt)
}

func isRunning(status string) bool {
	normalized := strings.ToLower(status)
	return strings.Contains(normalized, "running") || strings.Contains(normalized, "launch") || strings.Contains(normalized, "pending")
}

func isTerminal(status string) bool {
	normalized := strings.ToLower(status)
	return strings.Contains(normalized, "complete") || strings.Contains(normalized, "stopped") || strings.Contains(normalized, "disabled") || strings.Contains(normalized, "error") || strings.Contains(normalized, "skipped")
}

func ClientFor(options Options) processcompose.Client {
	return supervisor.Client(options.Layout, options.Runtime)
}
