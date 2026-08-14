//go:build darwin || linux

package resourceguard

import (
	"context"
	"fmt"
	"sort"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/procidentity"
	"github.com/jamesonstone/rungrid/internal/serviceexec"
	"github.com/jamesonstone/rungrid/internal/session"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func (w *worker) contain(ctx context.Context, monitor *serviceMonitor, observed observation) {
	now := time.Now()
	verified, err := w.revalidateCandidate(ctx, monitor, observed.candidate)
	if err != nil {
		monitor.authorityValid = false
		monitor.state = "enforcement_refused"
		monitor.degradedReason = err.Error()
		w.persistIncident(monitor, observed, "enforcement_refused", false, now)
		return
	}
	backoff, action := w.restartDecision(monitor, now)
	monitor.state = action
	w.persistIncident(monitor, observed, action, true, now)
	w.persistMonitor(monitor, now)
	_, _ = fmt.Fprintf(w.stdout, "[rungrid] resource guard: %s %s breach (%s); %s\n", monitor.service.Name, observed.tier, observed.trigger, action)

	grace := 2 * time.Second
	if observed.tier == "sustained" {
		grace = min(10*time.Second, w.manifest.Runtime.ShutdownTimeout.Duration)
	}
	stopContext, cancel := context.WithTimeout(ctx, grace)
	stopErr := w.client.Stop(stopContext, monitor.service.Name)
	cancel()
	if w.candidateAlive(ctx, verified) && monitor.service.Source == "compose" {
		composeContext, composeCancel := context.WithTimeout(ctx, grace)
		_ = serviceexec.ComposeShutdown(w.manifest, monitor.service, w.runtime.WorkspaceRoot, composeContext)
		composeCancel()
	}
	if w.candidateAlive(ctx, verified) {
		if escalationErr := w.escalate(ctx, monitor, verified); escalationErr != nil {
			monitor.state = "enforcement_refused"
			monitor.degradedReason = escalationErr.Error()
			w.persistMonitor(monitor, time.Now())
			return
		}
	}
	if stopErr != nil && w.candidateAlive(ctx, verified) {
		monitor.state = "stop_failed"
		monitor.degradedReason = "graceful stop and verified escalation did not stop the process tree"
		w.persistMonitor(monitor, time.Now())
		return
	}
	if action != "restart_pending" {
		monitor.emergencySamples = 0
		monitor.sustainedSince = time.Time{}
		w.persistMonitor(monitor, time.Now())
		return
	}
	if !waitBackoff(ctx, backoff) || shutdownStarted(w.layout, w.runtime.GenerationID) {
		monitor.state = "restart_suppressed"
		w.persistMonitor(monitor, time.Now())
		return
	}
	if err := w.revalidateRestart(ctx, monitor, verified); err != nil {
		monitor.state = "restart_refused"
		monitor.degradedReason = err.Error()
		w.persistMonitor(monitor, time.Now())
		return
	}
	if err := w.client.Start(ctx, monitor.service.Name); err != nil {
		monitor.state = "restart_failed"
		monitor.degradedReason = err.Error()
		w.persistMonitor(monitor, time.Now())
		return
	}
	monitor.state = "monitoring"
	monitor.emergencySamples = 0
	monitor.sustainedSince = time.Time{}
	monitor.degradedReason = ""
	w.persistMonitor(monitor, time.Now())
}

func (w *worker) revalidateCandidate(ctx context.Context, monitor *serviceMonitor, expected processCandidate) (processCandidate, error) {
	verifyContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := supervisor.Verify(verifyContext, w.layout, w.runtime); err != nil {
		return processCandidate{}, err
	}
	current, err := w.client.Get(verifyContext, monitor.service.Name)
	if err != nil || current.PID != expected.Root.PID {
		return processCandidate{}, fmt.Errorf("process Compose service identity changed before containment")
	}
	snapshotCtx, snapshotCancel := snapshotContext(verifyContext, w.manifest.Runtime.ResourceGuard.SampleInterval.Duration)
	snapshot, err := captureProcesses(snapshotCtx, w.hostMemory)
	snapshotCancel()
	if err != nil {
		return processCandidate{}, err
	}
	candidate, err := buildCandidate(snapshot, current.PID, w.runtime.PID, w.hostMemory)
	if err != nil || candidate.Root.StartIdentity != expected.Root.StartIdentity {
		return processCandidate{}, fmt.Errorf("managed process ancestry or start identity changed before containment")
	}
	return candidate, nil
}

func (w *worker) revalidateRestart(ctx context.Context, monitor *serviceMonitor, stopped processCandidate) error {
	verifyContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := supervisor.Verify(verifyContext, w.layout, w.runtime); err != nil {
		return err
	}
	if procidentity.Matches(stopped.Root.PID, stopped.Root.StartIdentity) {
		return fmt.Errorf("captured service root still exists before restart")
	}
	current, err := w.client.Get(verifyContext, monitor.service.Name)
	if err != nil {
		return err
	}
	if current.PID > 1 && isProcessRunning(current.Status) {
		return fmt.Errorf("process Compose reports a different running root before restart")
	}
	return nil
}

func (w *worker) escalate(ctx context.Context, monitor *serviceMonitor, expected processCandidate) error {
	verified, err := w.revalidateCandidate(ctx, monitor, expected)
	if err != nil {
		return err
	}
	sort.Ints(verified.Groups)
	for _, group := range verified.Groups {
		if err := syscall.Kill(-group, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
			return fmt.Errorf("signal verified process group %d: %w", group, err)
		}
	}
	if waitBackoff(ctx, 2*time.Second) {
		snapshotCtx, cancel := snapshotContext(ctx, w.manifest.Runtime.ResourceGuard.SampleInterval.Duration)
		snapshot, captureErr := captureProcesses(snapshotCtx, w.hostMemory)
		cancel()
		if captureErr != nil {
			return captureErr
		}
		for pid, original := range verified.Tree {
			current, exists := snapshot[pid]
			if !exists || current.StartIdentity != original.StartIdentity {
				continue
			}
			if err := syscall.Kill(pid, syscall.SIGKILL); err != nil && err != syscall.ESRCH {
				return fmt.Errorf("kill verified process %d: %w", pid, err)
			}
		}
	}
	return nil
}

func (w *worker) candidateAlive(ctx context.Context, candidate processCandidate) bool {
	if !procidentity.Matches(candidate.Root.PID, candidate.Root.StartIdentity) {
		return false
	}
	snapshotCtx, cancel := snapshotContext(ctx, w.manifest.Runtime.ResourceGuard.SampleInterval.Duration)
	snapshot, err := captureProcesses(snapshotCtx, w.hostMemory)
	cancel()
	if err != nil {
		return true
	}
	current, exists := snapshot[candidate.Root.PID]
	return exists && current.StartIdentity == candidate.Root.StartIdentity
}

func (w *worker) restartDecision(monitor *serviceMonitor, now time.Time) (time.Duration, string) {
	monitor.pruneRestarts(now)
	if len(monitor.restartHistory) >= monitor.policy.RestartLimit {
		monitor.circuitState = "open"
		return 0, "circuit_open"
	}
	if monitor.service.Activation == "tab" {
		if _, live := session.Active(w.layout, w.runtime.GenerationID, monitor.service.Name); !live {
			return 0, "stopped_no_owner"
		}
	}
	backoff := monitor.nextBackoff()
	monitor.restartHistory = append(monitor.restartHistory, now)
	return backoff, "restart_pending"
}

func (w *worker) persistIncident(monitor *serviceMonitor, observed observation, action string, valid bool, now time.Time) {
	summary := guardstate.IncidentSummary{
		OccurredAt: now.UTC().Format(time.RFC3339Nano), Subject: monitor.service.Name,
		Tier: observed.tier, Trigger: observed.trigger, Action: action, Metrics: observed.metrics,
	}
	summary.ID = state.Hash([]byte(summary.OccurredAt), []byte(summary.Subject), []byte(summary.Trigger))[:20]
	monitor.latestIncident = &summary
	incident := guardstate.Incident{
		IncidentSummary: summary, Scope: w.scope, RootPID: observed.candidate.Root.PID,
		RootIdentity: observed.candidate.Root.StartIdentity, Limits: observed.limits,
		RestartCount: len(monitor.restartHistory), CircuitState: monitor.circuitState, AuthorityValid: valid,
	}
	_ = guardstate.WriteIncident(w.layout, incident, w.manifest.Runtime.LogRetention.Duration)
}

func (w *worker) persistMonitor(monitor *serviceMonitor, now time.Time) {
	item := monitor.status(now)
	found := false
	for index := range w.status.Services {
		if w.status.Services[index].Name == item.Name {
			w.status.Services[index], found = item, true
			break
		}
	}
	if !found {
		w.status.Services = append(w.status.Services, item)
	}
	w.status.HeartbeatAt = now.UTC().Format(time.RFC3339Nano)
	_ = guardstate.WriteStatus(w.layout, w.status)
}

func waitBackoff(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
