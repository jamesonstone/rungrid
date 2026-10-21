package update

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// A pause failure must leave every running service as it was: services paused
// before the failure are restored, no ref moves, no unit runs, and nothing is
// recorded, so the next run retries the whole update.
func TestPauseFailureRestoresServicesAndChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.run(Options{Baseline: true})
	statePath := filepath.Join(f.layout.ProjectDir, storePath)
	recorded, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	f.push("backend/go.sum", "example.com/x v1.0.0 h1:x\n")
	f.push("frontend/package-lock.json", `{"lockfileVersion":3}`+"\n")
	before := f.head()
	f.coord.failPause = "backend"

	failed := f.run(Options{}).Repositories[0]
	if failed.Status != StatusFailed || !strings.Contains(failed.Reason, "pause backend") || !strings.Contains(failed.Reason, "nothing was changed") {
		t.Fatalf("repository = %s (%s)", failed.Status, failed.Reason)
	}
	if !reflect.DeepEqual(f.coord.paused, []string{"frontend"}) || !reflect.DeepEqual(f.coord.restarted, []string{"frontend"}) {
		t.Fatalf("paused %v restarted %v; frontend must be paused first and restored", f.coord.paused, f.coord.restarted)
	}
	if !f.coord.allRunning() {
		t.Fatalf("a pause failure left services stopped: %+v", f.coord.services)
	}
	if f.head() != before || len(f.exec.commands) != 0 {
		t.Fatalf("pause failure changed the checkout (%v) or ran %v", f.head() != before, f.exec.commands)
	}
	if got := serviceStatus(t, failed, "backend"); got.Status != StatusFailed {
		t.Fatalf("backend = %+v", got)
	}
	if got := serviceStatus(t, failed, "frontend"); got.Status != StatusSkipped || !strings.Contains(got.Reason, "left running") {
		t.Fatalf("frontend = %+v", got)
	}
	for _, step := range failed.Steps {
		if step.Status == StatusUpdated {
			t.Fatalf("step %s ran after a pause failure", step.ID)
		}
	}
	if after, _ := os.ReadFile(statePath); string(after) != string(recorded) {
		t.Fatal("pause failure recorded new fingerprints")
	}

	f.coord.failPause = ""
	retried := f.run(Options{}).Repositories[0]
	if retried.Status != StatusUpdated || !f.coord.allRunning() || len(f.exec.commands) != 2 {
		t.Fatalf("retry = %s, commands %v, running %v", retried.Status, f.exec.commands, f.coord.allRunning())
	}
	if !reflect.DeepEqual(f.coord.restarted, []string{"backend", "frontend"}) {
		t.Fatalf("retry restarted %v, want manifest order", f.coord.restarted)
	}
	again := f.run(Options{}).Repositories[0]
	if again.Status != StatusCurrent || len(f.exec.commands)+len(f.coord.paused) != 0 {
		t.Fatalf("third run was not a no-op: %+v", again)
	}
	for _, service := range again.Services {
		if service.Status != StatusCurrent {
			t.Fatalf("service %s = %s on a no-op run", service.Name, service.Status)
		}
	}
}

func TestDryRunNamesEachServiceRestartPath(t *testing.T) {
	f := newFixture(t)
	f.run(Options{Baseline: true})
	f.push("backend/go.sum", "example.com/x v1.0.0 h1:x\n")
	f.push("frontend/package-lock.json", `{"lockfileVersion":3}`+"\n")
	gitRun(t, f.primary, "fetch", "origin")
	plan := f.run(Options{DryRun: true}).Repositories[0]
	backend, frontend := serviceStatus(t, plan, "backend"), serviceStatus(t, plan, "frontend")
	if backend.RestartPath != RestartProcess || !strings.Contains(backend.Reason, "Process Compose") {
		t.Fatalf("backend plan = %+v", backend)
	}
	if frontend.RestartPath != RestartSession || !strings.Contains(frontend.Reason, "tab session") {
		t.Fatalf("frontend plan = %+v", frontend)
	}
	if len(f.coord.paused) != 0 {
		t.Fatalf("dry run paused %v", f.coord.paused)
	}
}

func TestRecoveredRestartIsReportedAsUpdated(t *testing.T) {
	f := newFixture(t)
	f.run(Options{Baseline: true})
	f.push("frontend/package-lock.json", `{"lockfileVersion":3}`+"\n")
	f.coord.recover = "frontend"
	result := f.run(Options{}).Repositories[0]
	frontend := serviceStatus(t, result, "frontend")
	if result.Status != StatusUpdated || frontend.Status != StatusUpdated {
		t.Fatalf("repository %s, frontend %+v", result.Status, frontend)
	}
	if frontend.RestartPath != RestartProcess || !strings.Contains(frontend.Reason, "restarted through Process Compose") {
		t.Fatalf("frontend did not report its fallback: %+v", frontend)
	}
	if again := f.run(Options{}).Repositories[0]; again.Status != StatusCurrent {
		t.Fatalf("after a recovered restart the next run = %s", again.Status)
	}
}
