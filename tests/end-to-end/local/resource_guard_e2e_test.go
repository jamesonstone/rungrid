//go:build darwin || linux

package local_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/guardstate"
	"github.com/jamesonstone/rungrid/internal/subprocess"
)

func TestResourceGuardEndToEnd(t *testing.T) {
	if os.Getenv("RUNGRID_E2E") != "1" {
		t.Skip("set RUNGRID_E2E=1 to run the real Process Compose resource guard")
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
	manual := exec.Command(hog, "--mode", "idle")
	manual.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := manual.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = syscall.Kill(-manual.Process.Pid, syscall.SIGKILL)
		_ = manual.Wait()
	})
	config := filepath.Join(workspace, ".rungrid.yaml")
	if err := os.WriteFile(config, []byte(resourceGuardManifest(manual.Process.Pid)), 0o600); err != nil {
		t.Fatal(err)
	}
	base := []string{"--config", config, "--state-dir", stateDirectory}
	runGuardCLI(t, binary, base, "up", "--headless", "--no-open")
	t.Cleanup(func() { _ = exec.Command(binary, append(base, "down")...).Run() })
	status := waitForGuardCircuits(t, binary, base)
	wants := map[string]string{"cpu": "cpu", "memory": "memory", "processes": "process_count", "threads": "thread_count"}
	for name, trigger := range wants {
		service := guardService(t, status, name)
		if service.RestartCount != 3 || service.CircuitState != "open" || service.LatestIncident == nil || service.LatestIncident.Trigger != trigger {
			t.Fatalf("%s circuit status: %#v", name, service)
		}
	}
	external := guardService(t, status, "manual-external")
	if external.Enforcement != "observe_only" || external.CircuitState == "open" {
		t.Fatalf("external process acquired enforcement: %#v", external)
	}
	assertProcessAlive(t, manual.Process.Pid, "manual/external process was terminated")
	runGuardCLI(t, binary, base, "start", "cpu", "--reset-resource-circuit")
	status = waitForGuardClosed(t, binary, base, "cpu")
	cpu := guardService(t, status, "cpu")
	if cpu.RestartCount != 0 || cpu.CircuitState != "closed" {
		t.Fatalf("explicit circuit reset did not persist: %#v", cpu)
	}
	managedPID := cpu.RootPID
	runGuardCLI(t, binary, base, "down")
	assertProcessAlive(t, manual.Process.Pid, "manual/external process was terminated by down")
	if managedPID > 1 && syscall.Kill(managedPID, 0) == nil {
		t.Fatalf("managed PID %d survived exact cleanup", managedPID)
	}
	inactive := readGuardStatus(t, binary, base)
	if inactive.ResourceGuard == nil || len(inactive.ResourceGuard.Services) == 0 {
		t.Fatal("persisted guard incidents disappeared with inactive runtime")
	}
}

func resourceGuardManifest(manualPID int) string {
	service := func(name, mode, override string) string {
		return fmt.Sprintf("  - name: %s\n    source: native\n    activation: workspace\n    run: {argv: [./resourcehog, --mode, %s, --counter, .%s-count, --breach-count, \"4\"]}\n%s", name, mode, name, override)
	}
	return `api_version: rungrid/v1
kind: Workspace
project: {name: Guard E2E, slug: guard-e2e, id: guard-e2e-k7m4q2}
terminal: {mode: headless, open: false}
runtime:
  startup_timeout: 45s
  shutdown_timeout: 5s
  resource_guard:
    sample_interval: 500ms
    emergency_window: 1500ms
    sustained_window: 30s
    learning_window: 1m
    restart_window: 1m
    backoff_initial: 500ms
    backoff_maximum: 2s
services:
` + service("cpu", "stubborn-cpu", "    resource_guard:\n      emergency: {cpu_percent: 0.5}\n      sustained: {cpu: {floor: 0.1, headroom: 0.1}}\n") +
		service("memory", "memory", "    resource_guard:\n      emergency: {memory_percent: 0.1}\n      sustained: {memory: {floor: 0.02, headroom: 0.02}}\n") +
		service("processes", "processes", "    resource_guard:\n      emergency: {processes: 4}\n      sustained: {processes: {floor: 2, headroom: 1}}\n") +
		service("threads", "threads", "    resource_guard:\n      emergency: {threads: 16}\n      sustained: {threads: {floor: 8, headroom: 4}}\n") +
		fmt.Sprintf("  - name: manual-external\n    source: external\n    activation: workspace\n    external: {command: {argv: [./resourcehog, --mode, probe, --pid, %s]}}\n", strconv.Itoa(manualPID))
}

type guardStatusData struct {
	Runtime       string             `json:"runtime"`
	ResourceGuard *guardstate.Status `json:"resource_guard"`
}

func readGuardStatus(t *testing.T, binary string, base []string) guardStatusData {
	t.Helper()
	command := exec.Command(binary, append(base, "--json", "status")...)
	output, err := subprocess.Combined(command)
	if err != nil {
		t.Fatalf("resource guard status: %v\n%s", err, output)
	}
	var envelope struct {
		Data guardStatusData `json:"data"`
	}
	if err := json.Unmarshal(output, &envelope); err != nil {
		t.Fatalf("decode resource guard status: %v\n%s", err, output)
	}
	return envelope.Data
}

func waitForGuardCircuits(t *testing.T, binary string, base []string) guardStatusData {
	t.Helper()
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		status := readGuardStatus(t, binary, base)
		if status.ResourceGuard != nil && status.ResourceGuard.Health == "healthy" {
			open := 0
			for _, service := range status.ResourceGuard.Services {
				if service.CircuitState == "open" {
					open++
				}
			}
			if open == 4 {
				return status
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatal("resource guard circuits did not open")
	return guardStatusData{}
}

func waitForGuardClosed(t *testing.T, binary string, base []string, name string) guardStatusData {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status := readGuardStatus(t, binary, base)
		service := guardService(t, status, name)
		if service.CircuitState == "closed" && service.RestartCount == 0 && service.RootPID > 1 {
			return status
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("resource circuit did not reset")
	return guardStatusData{}
}

func guardService(t *testing.T, status guardStatusData, name string) guardstate.ServiceStatus {
	t.Helper()
	if service, exists := lookupGuardService(status, name); exists {
		return service
	}
	t.Fatalf("resource guard service is missing: %s", name)
	return guardstate.ServiceStatus{}
}

func lookupGuardService(status guardStatusData, name string) (guardstate.ServiceStatus, bool) {
	if status.ResourceGuard != nil {
		for _, service := range status.ResourceGuard.Services {
			if service.Name == name {
				return service, true
			}
		}
	}
	return guardstate.ServiceStatus{}, false
}

func runGuardCLI(t *testing.T, binary string, base []string, arguments ...string) {
	t.Helper()
	command := exec.Command(binary, append(base, arguments...)...)
	if output, err := subprocess.Combined(command); err != nil {
		t.Fatalf("rungrid %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
}

func assertProcessAlive(t *testing.T, pid int, message string) {
	t.Helper()
	if syscall.Kill(pid, 0) != nil {
		t.Fatal(message)
	}
}
