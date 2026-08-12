//go:build darwin || linux

package local_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/subprocess"
)

func TestResourceGuardSustainedEndToEnd(t *testing.T) {
	if os.Getenv("RUNGRID_E2E") != "1" {
		t.Skip("set RUNGRID_E2E=1 to run the real Process Compose resource guard")
	}
	if os.Getenv("RUNGRID_E2E_SUSTAINED") != "1" {
		t.Skip("set RUNGRID_E2E_SUSTAINED=1 to run the two-minute sustained guard case")
	}
	if _, err := exec.LookPath("process-compose"); err != nil {
		t.Skip("Process Compose is unavailable")
	}
	directory := t.TempDir()
	binary, repositoryRoot := buildRungrid(t, directory)
	workspace := filepath.Join(directory, "workspace")
	stateDirectory := filepath.Join(directory, "state")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	hog := filepath.Join(workspace, "resourcehog")
	build := exec.Command("go", "build", "-o", hog, "./tests/fixtures/resourcehog")
	build.Dir = repositoryRoot
	if output, err := subprocess.Combined(build); err != nil {
		t.Fatalf("build resource fixture: %v\n%s", err, output)
	}
	trigger := filepath.Join(workspace, "increase-processes")
	configuration := fmt.Sprintf(`api_version: rungrid/v1
kind: Workspace
project: {name: Sustained E2E, slug: sustained-e2e, id: sustained-e2e-k7m4q2}
terminal: {mode: headless, open: false}
runtime:
  startup_timeout: 45s
  shutdown_timeout: 5s
  resource_guard:
    learning_window: 1m
    backoff_initial: 500ms
    backoff_maximum: 2s
services:
  - name: growing
    source: native
    activation: workspace
    run: {argv: [./resourcehog, --mode, switch-processes, --trigger, %q]}
`, trigger)
	config := filepath.Join(workspace, ".rungrid.yaml")
	if err := os.WriteFile(config, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--config", config, "--state-dir", stateDirectory}
	runGuardCLI(t, binary, base, "up", "--headless", "--no-open")
	t.Cleanup(func() { _ = exec.Command(binary, append(base, "down")...).Run() })
	waitForMatureBaseline(t, binary, base, "growing")
	triggeredAt := time.Now()
	if err := os.WriteFile(trigger, []byte("increase\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitForProcessGrowth(t, binary, base, "growing")
	incident := waitForSustainedIncident(t, binary, base, "growing")
	occurredAt, err := time.Parse(time.RFC3339Nano, incident.OccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := occurredAt.Sub(triggeredAt)
	if elapsed < 58*time.Second || elapsed > 62*time.Second {
		t.Fatalf("sustained containment occurred after %s, want 60s +/- two samples", elapsed)
	}
	runGuardCLI(t, binary, base, "down")
}

func waitForProcessGrowth(t *testing.T, binary string, base []string, serviceName string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var last guardstate.ServiceStatus
	for time.Now().Before(deadline) {
		status := readGuardStatus(t, binary, base)
		if service, exists := lookupGuardService(status, serviceName); exists {
			last = service
			if service.Metrics.Processes > 32 {
				return
			}
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("managed process tree did not grow: %#v", last)
}

func waitForMatureBaseline(t *testing.T, binary string, base []string, serviceName string) {
	t.Helper()
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) {
		status := readGuardStatus(t, binary, base)
		if service, exists := lookupGuardService(status, serviceName); exists && service.Baseline.Mature {
			return
		}
		time.Sleep(time.Second)
	}
	t.Fatal("resource baseline did not mature")
}

func waitForSustainedIncident(t *testing.T, binary string, base []string, serviceName string) guardstate.IncidentSummary {
	t.Helper()
	deadline := time.Now().Add(70 * time.Second)
	var last guardstate.ServiceStatus
	for time.Now().Before(deadline) {
		status := readGuardStatus(t, binary, base)
		service, exists := lookupGuardService(status, serviceName)
		if exists {
			last = service
		}
		if exists && service.LatestIncident != nil && service.LatestIncident.Tier == "sustained" && service.LatestIncident.Trigger == "process_count" {
			return *service.LatestIncident
		}
		time.Sleep(time.Second)
	}
	t.Fatalf("sustained resource incident was not observed: %#v", last)
	return guardstate.IncidentSummary{}
}
