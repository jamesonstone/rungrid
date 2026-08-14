package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/subprocess"
)

type workspaceStatus struct {
	ProjectID           string             `json:"project_id"`
	Runtime             string             `json:"runtime"`
	RuntimeVerification string             `json:"runtime_verification"`
	ResourceGuard       *guardstate.Status `json:"resource_guard"`
}

type sample struct {
	At         time.Time
	CPU        float64
	RSS        uint64
	SamplerMS  float64
	StateBytes uint64
	Restarts   int
	Circuits   int
}

type result struct {
	APIVersion      string   `json:"api_version"`
	Status          string   `json:"status"`
	ProjectID       string   `json:"project_id,omitempty"`
	GenerationID    string   `json:"generation_id,omitempty"`
	ManifestSHA256  string   `json:"effective_manifest_sha256,omitempty"`
	StartedAt       string   `json:"started_at"`
	CompletedAt     string   `json:"completed_at"`
	Duration        string   `json:"duration"`
	Samples         int      `json:"samples"`
	AverageGuardCPU float64  `json:"average_guard_cpu_percent"`
	P99GuardCPU     float64  `json:"p99_guard_cpu_percent"`
	MaximumRSSBytes uint64   `json:"maximum_guard_rss_bytes"`
	P99SamplerMS    float64  `json:"p99_sampler_duration_ms"`
	MaximumState    uint64   `json:"maximum_resource_guard_state_bytes"`
	RestartEvents   int      `json:"resource_restart_events"`
	CircuitOpenings int      `json:"circuit_openings"`
	Failures        []string `json:"failures"`
	cpuSamples      []float64
	samplerSamples  []float64
}

func runSoak(ctx context.Context, options options) (outcome result, returnErr error) {
	startedAt := time.Now()
	outcome = result{APIVersion: "rungrid/soak/v1", Status: "FAIL", StartedAt: startedAt.UTC().Format(time.RFC3339Nano), Duration: options.duration.String(), Failures: []string{}}
	if err := runLifecycle(ctx, options, "config", "validate"); err != nil {
		return outcome, err
	}
	status, err := readStatus(ctx, options)
	if err != nil {
		return outcome, err
	}
	startedRuntime := status.Runtime == "inactive"
	if startedRuntime {
		upArguments := []string{"up", "--headless", "--no-open"}
		if options.openWorkspace {
			upArguments = []string{"up"}
		}
		if err := runLifecycle(ctx, options, upArguments...); err != nil {
			return outcome, err
		}
		defer func() {
			downContext, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			if err := runLifecycle(downContext, options, "down"); err != nil && returnErr == nil {
				returnErr = fmt.Errorf("stop soak runtime: %w", err)
			}
		}()
	} else if status.Runtime != "active" {
		return outcome, fmt.Errorf("workspace runtime is %s", status.Runtime)
	}
	status, err = waitHealthy(ctx, options, 2*time.Minute, 30*time.Second)
	if err != nil {
		return outcome, err
	}
	outcome.ProjectID = status.ProjectID
	outcome.GenerationID = status.ResourceGuard.Scope.GenerationID
	outcome.ManifestSHA256 = status.ResourceGuard.Scope.EffectiveManifestSHA256
	if restarts, circuits := decisionCounts(status.ResourceGuard); restarts != 0 || circuits != 0 {
		return outcome, fmt.Errorf("soak requires a clean resource restart and circuit history")
	}
	layout, err := state.NewLayout(status.ProjectID, options.stateDir)
	if err != nil {
		return outcome, err
	}
	file, writer, err := openSamples(options.evidence)
	if err != nil {
		return outcome, err
	}
	defer func() { _ = file.Close() }()
	samplingStarted := time.Now()
	outcome.StartedAt = samplingStarted.UTC().Format(time.RFC3339Nano)
	deadline := samplingStarted.Add(options.duration)
	for time.Now().Before(deadline) {
		current, readErr := readStatus(ctx, options)
		if readErr != nil {
			return outcome, readErr
		}
		if problem := statusProblem(current); problem != "" {
			return outcome, fmt.Errorf("resource guard became unhealthy or lost authority: %s", problem)
		}
		heartbeat, heartbeatErr := time.Parse(time.RFC3339Nano, current.ResourceGuard.HeartbeatAt)
		if heartbeatErr != nil || time.Since(heartbeat) > 10*time.Second {
			return outcome, fmt.Errorf("resource guard heartbeat became stale")
		}
		if current.ResourceGuard.Scope.GenerationID != outcome.GenerationID || current.ResourceGuard.Scope.EffectiveManifestSHA256 != outcome.ManifestSHA256 {
			return outcome, fmt.Errorf("resource guard scope changed during soak")
		}
		restarts, circuits := decisionCounts(current.ResourceGuard)
		stateBytes, sizeErr := directorySize(filepath.Join(layout.ProjectDir, "resource-guard"))
		if sizeErr != nil {
			return outcome, sizeErr
		}
		item := sample{At: time.Now(), CPU: current.ResourceGuard.GuardCPUPercent, RSS: current.ResourceGuard.GuardRSSBytes, SamplerMS: current.ResourceGuard.SamplerDurationMS, StateBytes: stateBytes, Restarts: restarts, Circuits: circuits}
		if err := writeSample(writer, item); err != nil {
			return outcome, err
		}
		outcome.add(item)
		if restarts != 0 || circuits != 0 {
			return outcome, fmt.Errorf("healthy soak observed resource restart or circuit activity")
		}
		if err := waitUntil(ctx, min(options.interval, time.Until(deadline))); err != nil {
			return outcome, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return outcome, err
	}
	outcome.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	outcome.evaluate()
	if len(outcome.Failures) > 0 {
		return outcome, fmt.Errorf("soak acceptance failed")
	}
	outcome.Status = "PASS"
	return outcome, nil
}

func runLifecycle(ctx context.Context, options options, arguments ...string) error {
	base := []string{"--config", options.config}
	if options.stateDir != "" {
		base = append(base, "--state-dir", options.stateDir)
	}
	command := exec.CommandContext(ctx, options.rungrid, append(base, arguments...)...)
	command.Dir = filepath.Dir(options.config)
	_, err := subprocess.Combined(command)
	if err != nil {
		return fmt.Errorf("rungrid lifecycle command failed (output redacted): %w", err)
	}
	return nil
}

func readStatus(ctx context.Context, options options) (workspaceStatus, error) {
	base := []string{"--config", options.config}
	if options.stateDir != "" {
		base = append(base, "--state-dir", options.stateDir)
	}
	command := exec.CommandContext(ctx, options.rungrid, append(base, "--json", "status")...)
	command.Dir = filepath.Dir(options.config)
	capture, err := subprocess.Run(command)
	if err != nil {
		return workspaceStatus{}, fmt.Errorf("read Rungrid status (output redacted): %w", err)
	}
	var envelope struct {
		Data workspaceStatus `json:"data"`
	}
	if err := json.Unmarshal(capture.Stdout, &envelope); err != nil {
		return workspaceStatus{}, err
	}
	return envelope.Data, nil
}

func waitHealthy(ctx context.Context, options options, maximum, stableFor time.Duration) (workspaceStatus, error) {
	deadline := time.Now().Add(maximum)
	var healthySince time.Time
	var last workspaceStatus
	for time.Now().Before(deadline) {
		status, err := readStatus(ctx, options)
		if err == nil {
			last = status
		}
		if err == nil && statusProblem(status) == "" {
			if healthySince.IsZero() {
				healthySince = time.Now()
			}
			if time.Since(healthySince) >= stableFor {
				return status, nil
			}
		} else {
			healthySince = time.Time{}
		}
		if err := waitUntil(ctx, time.Second); err != nil {
			return workspaceStatus{}, err
		}
	}
	return workspaceStatus{}, fmt.Errorf("resource guard did not remain healthy: %s", statusProblem(last))
}

func statusProblem(status workspaceStatus) string {
	if status.Runtime != "active" {
		if status.RuntimeVerification != "" {
			return fmt.Sprintf("runtime=%s verification=%s", status.Runtime, status.RuntimeVerification)
		}
		return "runtime=" + status.Runtime
	}
	if status.ResourceGuard == nil {
		return "guard status unavailable"
	}
	if status.ResourceGuard.Health != "healthy" || !status.ResourceGuard.AuthorityValid {
		return fmt.Sprintf("health=%s authority_valid=%t reason=%s", status.ResourceGuard.Health, status.ResourceGuard.AuthorityValid, status.ResourceGuard.DegradedReason)
	}
	return ""
}

func decisionCounts(status *guardstate.Status) (int, int) {
	restarts, circuits := 0, 0
	for _, service := range status.Services {
		restarts += service.RestartCount
		if service.CircuitState == "open" {
			circuits++
		}
	}
	return restarts, circuits
}

func waitUntil(ctx context.Context, duration time.Duration) error {
	if duration <= 0 {
		return nil
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
