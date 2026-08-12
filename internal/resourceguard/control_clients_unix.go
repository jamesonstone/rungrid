//go:build darwin || linux

package resourceguard

import (
	"context"
	"fmt"
	"math"
	"sort"
	"syscall"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/procidentity"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func (w *worker) observeControlClients(ctx context.Context, now time.Time, snapshot processSnapshot) error {
	registrations, err := guardstate.ListControlClients(w.layout, w.scope)
	if err != nil {
		return err
	}
	seen := map[int]bool{}
	limits := emergencyLimits(w.manifest.Runtime.ResourceGuard)
	needed := int(math.Ceil(float64(w.manifest.Runtime.ResourceGuard.EmergencyWindow.Duration) /
		float64(w.manifest.Runtime.ResourceGuard.SampleInterval.Duration)))
	for _, registration := range registrations {
		seen[registration.PID] = true
		candidate, candidateErr := buildControlCandidate(snapshot, registration, w.hostMemory)
		if candidateErr != nil {
			w.controlBreaches[registration.PID] = 0
			if controlClientRegistrationStale(snapshot, registration) {
				_ = guardstate.RemoveControlClient(w.layout, registration)
			}
			continue
		}
		trigger := breach(candidate.Metrics, limits)
		if deadlineExpired(registration, now) {
			trigger = "hung_command"
		}
		if trigger == "" {
			w.controlBreaches[registration.PID] = 0
			continue
		}
		w.controlBreaches[registration.PID]++
		if w.controlBreaches[registration.PID] < needed {
			continue
		}
		w.containControlClient(ctx, now, registration, candidate, trigger, limits)
		w.controlBreaches[registration.PID] = 0
	}
	for pid := range w.controlBreaches {
		if !seen[pid] {
			delete(w.controlBreaches, pid)
		}
	}
	return nil
}

func controlClientRegistrationStale(snapshot processSnapshot, registration guardstate.ControlClient) bool {
	process, exists := snapshot[registration.PID]
	return !exists || process.StartIdentity != registration.ProcessIdentity
}

func (w *worker) containControlClient(
	ctx context.Context,
	now time.Time,
	registration guardstate.ControlClient,
	expected processCandidate,
	trigger string,
	limits guardstate.Limits,
) {
	candidate, err := w.revalidateControlClient(ctx, registration, expected)
	action := "terminated"
	valid := err == nil
	if err != nil {
		action = "enforcement_refused"
	} else {
		sort.Ints(candidate.Groups)
		for _, group := range candidate.Groups {
			if signalErr := syscall.Kill(-group, syscall.SIGTERM); signalErr != nil && signalErr != syscall.ESRCH {
				action = "termination_failed"
				break
			}
		}
		if action == "terminated" && waitBackoff(ctx, 2*time.Second) {
			for pid, original := range candidate.Tree {
				if procidentity.Matches(pid, original.StartIdentity) {
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			}
		}
	}
	summary := guardstate.IncidentSummary{
		OccurredAt: now.UTC().Format(time.RFC3339Nano), Subject: "process-compose-client",
		Tier: "emergency", Trigger: trigger, Action: action, Metrics: expected.Metrics,
	}
	summary.ID = state.Hash([]byte(summary.OccurredAt), []byte(fmt.Sprintf("%d", registration.PID)), []byte(trigger))[:20]
	w.status.LatestControlIncident = &summary
	incident := guardstate.Incident{
		IncidentSummary: summary, Scope: w.scope, RootPID: registration.PID,
		RootIdentity: registration.ProcessIdentity, Limits: limits, AuthorityValid: valid,
		CircuitState: "not_applicable",
	}
	_ = guardstate.WriteIncident(w.layout, incident, w.manifest.Runtime.LogRetention.Duration)
	_, _ = fmt.Fprintf(w.stdout, "[rungrid] resource guard: Process Compose %s client %s; %s\n", registration.Operation, trigger, action)
}

func (w *worker) revalidateControlClient(
	ctx context.Context,
	registration guardstate.ControlClient,
	expected processCandidate,
) (processCandidate, error) {
	verifyContext, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if err := supervisor.Verify(verifyContext, w.layout, w.runtime); err != nil {
		return processCandidate{}, err
	}
	registrations, err := guardstate.ListControlClients(w.layout, w.scope)
	if err != nil {
		return processCandidate{}, err
	}
	registered := false
	for _, current := range registrations {
		if current == registration {
			registered = true
			break
		}
	}
	if !registered {
		return processCandidate{}, fmt.Errorf("control client registration changed before containment")
	}
	snapshotCtx, snapshotCancel := snapshotContext(verifyContext, w.manifest.Runtime.ResourceGuard.SampleInterval.Duration)
	snapshot, err := captureProcesses(snapshotCtx, w.hostMemory)
	snapshotCancel()
	if err != nil {
		return processCandidate{}, err
	}
	candidate, err := buildControlCandidate(snapshot, registration, w.hostMemory)
	if err != nil || candidate.Root.StartIdentity != expected.Root.StartIdentity {
		return processCandidate{}, fmt.Errorf("control client identity changed before containment")
	}
	return candidate, nil
}

func deadlineExpired(registration guardstate.ControlClient, now time.Time) bool {
	if registration.DeadlineAt == "" {
		return false
	}
	deadline, err := time.Parse(time.RFC3339Nano, registration.DeadlineAt)
	return err != nil || now.After(deadline)
}
