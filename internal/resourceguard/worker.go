//go:build darwin || linux

package resourceguard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/serviceexec"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

type WorkerOptions struct {
	RuntimeContext serviceexec.Context
	Stdout         io.Writer
}

type worker struct {
	layout          state.Layout
	runtime         supervisor.Runtime
	scope           guardstate.AuthorityScope
	manifest        *manifest.Manifest
	client          processcompose.Client
	hostMemory      uint64
	stdout          io.Writer
	monitors        map[string]*serviceMonitor
	controlBreaches map[int]int
	status          guardstate.Status
}

func Run(ctx context.Context, options WorkerOptions) error {
	runtimeState, err := awaitRuntime(ctx, options.RuntimeContext)
	if err != nil {
		return err
	}
	hostMemory, err := hostMemoryBytes()
	if err != nil {
		return err
	}
	scope := supervisor.AuthorityScope(options.RuntimeContext.Layout, runtimeState)
	w := &worker{
		layout: options.RuntimeContext.Layout, runtime: runtimeState, scope: scope,
		manifest: options.RuntimeContext.Manifest, client: supervisor.Client(options.RuntimeContext.Layout, runtimeState),
		hostMemory: hostMemory, stdout: options.Stdout, monitors: map[string]*serviceMonitor{}, controlBreaches: map[int]int{},
		status: guardstate.Status{
			ProjectID: scope.ProjectID, GenerationID: scope.GenerationID, Scope: scope,
			AuthorityValid: true, Health: "starting", Services: []guardstate.ServiceStatus{},
		},
	}
	if err := w.initialize(); err != nil {
		return err
	}
	ticker := time.NewTicker(w.manifest.Runtime.ResourceGuard.SampleInterval.Duration)
	defer ticker.Stop()
	if err := w.sample(ctx); err != nil {
		w.degrade(err)
	}
	for {
		select {
		case <-ctx.Done():
			w.status.Health = "inactive"
			w.status.AuthorityValid = false
			w.status.HeartbeatAt = state.RuntimeTimestamp()
			_ = guardstate.WriteStatus(w.layout, w.status)
			return nil
		case <-ticker.C:
			if shutdownStarted(w.layout, w.runtime.GenerationID) {
				w.status.Shutdown = true
				w.status.Health = "inactive"
				w.status.AuthorityValid = false
				w.status.HeartbeatAt = state.RuntimeTimestamp()
				_ = guardstate.WriteStatus(w.layout, w.status)
				return nil
			}
			if err := w.sample(ctx); err != nil {
				w.degrade(err)
			}
		}
	}
}

func awaitRuntime(ctx context.Context, runtimeContext serviceexec.Context) (supervisor.Runtime, error) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		runtimeState, err := supervisor.Read(runtimeContext.Layout)
		if err == nil && runtimeState.GenerationID == runtimeContext.GenerationID {
			if verifyErr := supervisor.Verify(ctx, runtimeContext.Layout, runtimeState); verifyErr == nil {
				return runtimeState, nil
			}
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return supervisor.Runtime{}, err
		}
		select {
		case <-ctx.Done():
			return supervisor.Runtime{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func (w *worker) initialize() error {
	if err := guardstate.PruneExitedControlClients(w.layout); err != nil {
		return err
	}
	previous, hasPrevious, err := guardstate.ReadStatus(w.layout)
	if err != nil {
		return err
	}
	for index := range w.manifest.Services {
		service := &w.manifest.Services[index]
		policy := manifest.EffectiveServiceResourceGuard(w.manifest.Runtime.ResourceGuard, service)
		identity := serviceIdentity(w.scope, service)
		monitor := &serviceMonitor{service: service, policy: policy, identity: identity}
		baseline, err := loadBaselineTracker(w.layout, w.scope, service.Name, identity, policy.LearningWindow.Duration)
		if err != nil {
			return err
		}
		monitor.baseline = baseline
		if hasPrevious && guardstate.SameEffectiveGeneration(previous.Scope, w.scope) {
			monitor.restore(previous)
		}
		w.monitors[service.Name] = monitor
	}
	return nil
}

func (w *worker) sample(ctx context.Context) error {
	interval := w.manifest.Runtime.ResourceGuard.SampleInterval.Duration
	samplerStarted := time.Now()
	snapshotCtx, cancel := snapshotContext(ctx, interval)
	snapshot, err := captureProcesses(snapshotCtx, w.hostMemory)
	cancel()
	if err != nil {
		return err
	}
	w.status.SamplerDurationMS = float64(time.Since(samplerStarted).Microseconds()) / 1000
	if _, exists := snapshot[os.Getpid()]; exists {
		w.status.GuardPID = os.Getpid()
		w.status.GuardCPUPercent, w.status.GuardRSSBytes, w.status.GuardThreads = treeOverhead(snapshot, os.Getpid())
	}
	if !w.sampleScopeValid(snapshot) {
		w.status.AuthorityValid = false
		return fmt.Errorf("resource guard authority scope no longer matches")
	}
	states, _, err := w.client.List(ctx)
	if err != nil {
		return fmt.Errorf("resource guard Process Compose state query failed: %w", err)
	}
	byName := make(map[string]processcompose.ProcessState, len(states))
	for _, current := range states {
		byName[current.Name] = current
	}
	now := time.Now()
	statuses := make([]guardstate.ServiceStatus, 0, len(w.manifest.Services))
	for index := range w.manifest.Services {
		service := &w.manifest.Services[index]
		monitor := w.monitors[service.Name]
		reset, resetErr := guardstate.ConsumeCircuitReset(w.layout, w.scope, service.Name)
		if resetErr != nil {
			return resetErr
		}
		if reset {
			monitor.circuitState = "closed"
			monitor.restartHistory = nil
			monitor.state = "monitoring"
		}
		status := monitor.observe(now, interval, snapshot, byName[service.Name], w.runtime.PID, w.hostMemory)
		if status.trigger != "" {
			w.contain(ctx, monitor, status)
		}
		if err := monitor.baseline.Checkpoint(now); err != nil {
			return err
		}
		statuses = append(statuses, monitor.status(now))
	}
	if err := w.observeControlClients(ctx, now, snapshot); err != nil {
		return err
	}
	w.status.Services = statuses
	w.status.Health = "healthy"
	w.status.AuthorityValid = true
	w.status.DegradedReason = ""
	w.status.HeartbeatAt = now.UTC().Format(time.RFC3339Nano)
	return guardstate.WriteStatus(w.layout, w.status)
}

func (w *worker) sampleScopeValid(snapshot processSnapshot) bool {
	if !supervisor.StaticScopeMatches(w.layout, w.runtime) {
		return false
	}
	runtimeProcess, exists := snapshot[w.runtime.PID]
	return exists && runtimeProcess.StartIdentity == w.runtime.ProcessIdentity
}

func (w *worker) degrade(err error) {
	w.status.Health = "degraded"
	w.status.DegradedReason = err.Error()
	w.status.HeartbeatAt = state.RuntimeTimestamp()
	_ = guardstate.WriteStatus(w.layout, w.status)
}

func serviceIdentity(scope guardstate.AuthorityScope, service *manifest.Service) string {
	content, _ := json.Marshal(service)
	return state.Hash([]byte(scope.ProjectID), []byte(scope.GenerationID), []byte(scope.EffectiveManifestSHA256), content)
}

func shutdownStarted(layout state.Layout, generationID string) bool {
	_, err := os.Lstat(filepath.Join(layout.ProjectDir, "locks", "down-"+generationID+".json"))
	return err == nil || !os.IsNotExist(err)
}

func healthyState(service *manifest.Service, current processcompose.ProcessState) bool {
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
