package resourceguard

import (
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
)

func TestExternalServiceIsAlwaysObservationOnly(t *testing.T) {
	service := &manifest.Service{Name: "postgres", Source: "external", Activation: "workspace"}
	monitor := &serviceMonitor{service: service, policy: manifest.DefaultResourceGuard()}
	observed := monitor.observe(
		time.Now(), time.Second,
		processSnapshot{20: {PID: 20, PPID: 1, PGID: 20, StartIdentity: "manual", CPUPercent: 10_000, Threads: 100_000}},
		processcompose.ProcessState{Name: "postgres", Status: "Running", PID: 20}, 10, 1,
	)
	if observed.trigger != "" || monitor.state != "observe_only" || monitor.authorityValid {
		t.Fatalf("external service acquired enforcement authority: %#v, %#v", observed, monitor)
	}
}

func TestMatureBaselineDoesNotLearnSustainedBreach(t *testing.T) {
	policy := manifest.DefaultResourceGuard()
	service := &manifest.Service{Name: "api", Source: "native", Activation: "workspace"}
	baseline := &baselineTracker{
		service: "api", identity: "identity", learningWindow: time.Minute,
		healthyDuration: time.Minute, healthySamples: 60,
		persistedP99: guardstate.Metrics{Processes: 1, Threads: 1},
	}
	monitor := &serviceMonitor{service: service, policy: policy, baseline: baseline}
	snapshot := processSnapshot{
		10: {PID: 10, PPID: 1, PGID: 10, StartIdentity: "runtime"},
		20: {PID: 20, PPID: 10, PGID: 20, StartIdentity: "root", Threads: 1},
	}
	for index := 0; index < 40; index++ {
		pid := 100 + index
		snapshot[pid] = processInfo{PID: pid, PPID: 20, PGID: 20, StartIdentity: "child", Threads: 1}
	}
	monitor.observe(time.Now(), time.Second, snapshot, processcompose.ProcessState{Name: "api", Status: "Running", PID: 20}, 10, 1<<30)
	if baseline.healthySamples != 60 || monitor.effective.Processes != 32 {
		t.Fatalf("mature baseline learned its breach: samples=%d limits=%#v", baseline.healthySamples, monitor.effective)
	}
}

func TestFourthResourceBreachOpensCircuitAfterTwoFourEightBackoffs(t *testing.T) {
	policy := manifest.DefaultResourceGuard()
	service := &manifest.Service{Name: "api", Source: "native", Activation: "workspace"}
	monitor := &serviceMonitor{service: service, policy: policy}
	worker := &worker{}
	now := time.Now()
	for index, expected := range []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second} {
		backoff, action := worker.restartDecision(monitor, now.Add(time.Duration(index)*time.Minute))
		if action != "restart_pending" || backoff != expected {
			t.Fatalf("restart %d: action=%s backoff=%s, want restart_pending %s", index+1, action, backoff, expected)
		}
	}
	if backoff, action := worker.restartDecision(monitor, now.Add(3*time.Minute)); action != "circuit_open" || backoff != 0 {
		t.Fatalf("fourth breach did not open circuit: action=%s backoff=%s", action, backoff)
	}
	if monitor.circuitState != "open" || len(monitor.restartHistory) != 3 {
		t.Fatalf("unexpected circuit history: %#v", monitor)
	}
}
