package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/update"
)

// fakeProcess models the Process Compose behavior behind the field failure:
// stop returns at once while the process stays Terminating for a few polls,
// and a start sent during that window is rejected as already running.
type fakeProcess struct {
	status        string
	terminating   int
	rejected      int
	starts        int
	stopErr       error
	startFailures int
}

func (p *fakeProcess) Get(context.Context, string) (processcompose.ProcessState, error) {
	if p.status == "Terminating" {
		if p.terminating == 0 {
			p.status = "Completed"
		} else {
			p.terminating--
		}
	}
	return processcompose.ProcessState{Status: p.status}, nil
}

func (p *fakeProcess) Start(context.Context, string) error {
	if !processcompose.Settled(p.status) {
		p.rejected++
		return errors.New("Process Compose start process returned HTTP 400 (process is already running)")
	}
	if p.startFailures > 0 {
		p.startFailures--
		return errors.New("Process Compose start process returned HTTP 500")
	}
	p.starts++
	p.status = "Running"
	return nil
}

func (p *fakeProcess) Stop(context.Context, string) error {
	if p.stopErr != nil {
		return p.stopErr
	}
	p.status, p.terminating = "Terminating", 3
	return nil
}

func restartFor(process *fakeProcess, sessionPause func(context.Context) (func(context.Context) error, error)) serviceRestart {
	return serviceRestart{
		name: "backend", process: process, sessionPause: sessionPause,
		ready:       func(current processcompose.ProcessState) bool { return current.Status == "Running" },
		stopTimeout: 2 * time.Second, startTimeout: 2 * time.Second,
	}
}

func recoveredDetail(t *testing.T, err error) string {
	t.Helper()
	var recovered update.Recovered
	if !errors.As(err, &recovered) {
		t.Fatalf("resume error = %v, want a recovered restart", err)
	}
	return recovered.Detail
}

// The field failure: the session paused, then its resume failed (a start that
// raced termination, or a broken session socket) and left the process
// Completed. The restart must fall back to Process Compose and leave it running.
func TestBrokenSessionResumeFallsBackToProcessCompose(t *testing.T) {
	process := &fakeProcess{status: "Running"}
	restart := restartFor(process, func(ctx context.Context) (func(context.Context) error, error) {
		process.status = "Completed"
		return func(context.Context) error {
			return errors.New("write unix ->runtime.sock: write: broken pipe")
		}, nil
	})
	resume, err := restart.pause(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	detail := recoveredDetail(t, resume(context.Background()))
	if process.status != "Running" || !strings.Contains(detail, "session resume failed") || !strings.Contains(detail, "Process Compose") {
		t.Fatalf("status %s detail %q", process.status, detail)
	}
}

func TestStaleSessionPauseFallsBackToProcessCompose(t *testing.T) {
	process := &fakeProcess{status: "Running"}
	restart := restartFor(process, func(context.Context) (func(context.Context) error, error) {
		return nil, errors.New("tab-owned service has no live owning session")
	})
	resume, err := restart.pause(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !processcompose.Settled(process.status) {
		t.Fatalf("pause returned while the process was %s", process.status)
	}
	detail := recoveredDetail(t, resume(context.Background()))
	if process.status != "Running" || process.rejected != 0 || !strings.Contains(detail, "session pause failed") {
		t.Fatalf("status %s rejected starts %d detail %q", process.status, process.rejected, detail)
	}
}

func TestFailedPauseNeverLeavesTheServiceStopped(t *testing.T) {
	process := &fakeProcess{status: "Running", stopErr: errors.New("Process Compose stop process returned HTTP 500")}
	restart := restartFor(process, func(context.Context) (func(context.Context) error, error) {
		process.status = "Completed"
		return nil, errors.New("wait for service maintenance paused: context deadline exceeded")
	})
	if _, err := restart.pause(context.Background()); err == nil {
		t.Fatal("pause succeeded although neither the session nor Process Compose could stop the service")
	}
	if process.status != "Running" {
		t.Fatalf("a failed pause left the service %s", process.status)
	}
}

func TestProcessComposeRestartWaitsOutTermination(t *testing.T) {
	process := &fakeProcess{status: "Running"}
	resume, err := restartFor(process, nil).pause(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if process.status != "Running" || process.rejected != 0 || process.starts != 1 {
		t.Fatalf("status %s rejected %d starts %d", process.status, process.rejected, process.starts)
	}
}

func TestEnsureRunningAdoptsAndRetries(t *testing.T) {
	running := &fakeProcess{status: "Running"}
	if err := restartFor(running, nil).ensureRunning(context.Background()); err != nil || running.starts != 0 {
		t.Fatalf("running process: err %v starts %d", err, running.starts)
	}
	flaky := &fakeProcess{status: "Skipped", startFailures: 2}
	if err := restartFor(flaky, nil).ensureRunning(context.Background()); err != nil || flaky.status != "Running" {
		t.Fatalf("flaky start: err %v status %s", err, flaky.status)
	}
	broken := &fakeProcess{status: "Completed", startFailures: 1000}
	restart := restartFor(broken, nil)
	restart.stopTimeout, restart.startTimeout = 300*time.Millisecond, 300*time.Millisecond
	if err := restart.ensureRunning(context.Background()); err == nil || !strings.Contains(err.Error(), "could not be started") {
		t.Fatalf("unstartable process: err %v", err)
	}
}
