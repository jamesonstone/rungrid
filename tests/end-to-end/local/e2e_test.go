//go:build darwin || linux

package local_test

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/subprocess"
)

func TestHeadlessLifecycleEndToEnd(t *testing.T) {
	if os.Getenv("RUNGRID_E2E") != "1" {
		t.Skip("set RUNGRID_E2E=1 to run the real Process Compose lifecycle")
	}
	if _, err := exec.LookPath("process-compose"); err != nil {
		t.Skip("Process Compose is unavailable")
	}
	directory := t.TempDir()
	binary, repositoryRoot := buildRungrid(t, directory)
	stateDirectory := filepath.Join(directory, "state")
	runtimePath := filepath.Join(stateDirectory, "rungrid", "projects", "headless-example-r5n2w7", "runtime.json")
	t.Setenv("RUNGRID_TEST_RUNTIME_PATH", runtimePath)
	workspace := filepath.Join(directory, "workspace")
	control := filepath.Join(workspace, "control")
	for _, path := range []string{control, filepath.Join(workspace, "api"), filepath.Join(workspace, "web")} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	hook := filepath.Join(workspace, "lifecycle-hook")
	hookContent := `#!/bin/sh
if [ "$1" = "after" ] && [ -e "$RUNGRID_TEST_RUNTIME_PATH" ]; then
  printf 'after-before-runtime-stop\n' >> lifecycle-events
  exit 9
fi
printf '%s\n' "$1" >> lifecycle-events
`
	if err := os.WriteFile(hook, []byte(hookContent), 0o700); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join(repositoryRoot, "testdata", "headless", ".rungrid.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fixture = bytes.Replace(fixture, []byte("runtime:\n"), []byte(`workspace:
  root: ..
lifecycle:
  before_up:
    - name: prepare
      run: {argv: [./lifecycle-hook, before]}
  after_down:
    - name: cleanup
      run: {argv: [./lifecycle-hook, after]}
runtime:
`), 1)
	config := filepath.Join(control, ".rungrid.yaml")
	fixture = bytes.Replace(fixture, []byte("mode: headless"), []byte("mode: warp"), 1)
	if err := os.WriteFile(config, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	baseArguments := []string{"--config", config, "--state-dir", stateDirectory}
	run := func(arguments ...string) []byte {
		t.Helper()
		command := exec.Command(binary, append(baseArguments, arguments...)...)
		output, err := subprocess.Combined(command)
		if err != nil {
			t.Fatalf("rungrid %s: %v\n%s", strings.Join(arguments, " "), err, output)
		}
		return output
	}
	plan := run("--json", "plan")
	if bytes.Contains(plan, []byte(workspace)) {
		t.Fatalf("portable plan contains temporary absolute workspace path: %s", plan)
	}
	run("up", "--headless", "--no-open")
	t.Cleanup(func() { _ = exec.Command(binary, append(baseArguments, "down")...).Run() })
	run("up", "--headless", "--no-open")
	assertE2ELines(t, filepath.Join(workspace, "lifecycle-events"), []string{"before"})
	runtimeRecord, err := os.ReadFile(runtimePath)
	if err != nil {
		t.Fatal(err)
	}
	var runtimeIdentity struct {
		GenerationID string `json:"generation_id"`
	}
	if err := json.Unmarshal(runtimeRecord, &runtimeIdentity); err != nil {
		t.Fatal(err)
	}
	terminalDirectory := filepath.Join(stateDirectory, "rungrid", "projects", "headless-example-r5n2w7", "generations", runtimeIdentity.GenerationID, "terminal")
	if _, err := os.Stat(terminalDirectory); !os.IsNotExist(err) {
		t.Fatalf("--headless generated graphical terminal files: %v", err)
	}
	var tampered map[string]any
	if err := json.Unmarshal(runtimeRecord, &tampered); err != nil {
		t.Fatal(err)
	}
	tampered["pid"] = os.Getpid()
	tamperedContent, _ := json.MarshalIndent(tampered, "", "  ")
	if err := os.WriteFile(runtimePath, append(tamperedContent, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := subprocess.Combined(exec.Command(binary, append(baseArguments, "status")...)); err != nil || !strings.Contains(string(output), "runtime degraded") || !strings.Contains(string(output), "runtime PID") {
		t.Fatalf("tampered runtime PID was not reported as degraded: err=%v output=%s", err, output)
	}
	if err := os.WriteFile(runtimePath, runtimeRecord, 0o600); err != nil {
		t.Fatal(err)
	}
	overlay := filepath.Join(control, ".rungrid.local.yaml")
	if err := os.WriteFile(overlay, []byte("api_version: rungrid/v1\nkind: Workspace\nruntime:\n  startup_timeout: 11s\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changedArguments := []string{"--config", config, "--local", overlay, "--state-dir", stateDirectory, "generate"}
	if output, err := subprocess.Combined(exec.Command(binary, changedArguments...)); err == nil || !strings.Contains(string(output), "requires lifecycle cleanup") {
		t.Fatalf("active-generation guard did not fail closed: err=%v output=%s", err, output)
	}
	if err := os.Remove(overlay); err != nil {
		t.Fatal(err)
	}

	session := exec.Command(binary, append(baseArguments, "session", "worker")...)
	var sessionOutput bytes.Buffer
	session.Stdout = &sessionOutput
	session.Stderr = &sessionOutput
	if err := session.Start(); err != nil {
		t.Fatal(err)
	}
	waitForE2EState(t, binary, baseArguments, "worker", "Running")
	duplicate := exec.Command(binary, append(baseArguments, "session", "worker")...)
	if output, err := subprocess.Combined(duplicate); err == nil || !strings.Contains(string(output), "already has an owning session") {
		t.Fatalf("duplicate session was not rejected: err=%v output=%s", err, output)
	}
	if err := session.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	if err := session.Wait(); err == nil {
		t.Fatal("interrupted session unexpectedly returned success")
	}
	waitForE2EState(t, binary, baseArguments, "worker", "Completed")

	restarted := exec.Command(binary, append(baseArguments, "session", "worker")...)
	var restartedOutput bytes.Buffer
	restarted.Stdout = &restartedOutput
	restarted.Stderr = &restartedOutput
	if err := restarted.Start(); err != nil {
		t.Fatal(err)
	}
	waitForE2EState(t, binary, baseArguments, "worker", "Running")
	run("down")
	if err := restarted.Wait(); err != nil || !strings.Contains(restartedOutput.String(), "session released: workspace shutdown began") {
		t.Fatalf("matching session did not quiesce during down: err=%v output=%s", err, restartedOutput.String())
	}
	assertE2ELines(t, filepath.Join(workspace, "lifecycle-events"), []string{"before", "after"})
	run("down")
	assertE2ELines(t, filepath.Join(workspace, "lifecycle-events"), []string{"before", "after"})
	if _, err := os.Stat(filepath.Join(stateDirectory, "rungrid", "projects", "headless-example-r5n2w7", "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("runtime record remains after down: %v", err)
	}
}

func TestTabOnlyLifecycleEndToEnd(t *testing.T) {
	if os.Getenv("RUNGRID_E2E") != "1" {
		t.Skip("set RUNGRID_E2E=1 to run the real Process Compose lifecycle")
	}
	if _, err := exec.LookPath("process-compose"); err != nil {
		t.Skip("Process Compose is unavailable")
	}
	directory := t.TempDir()
	binary, _ := buildRungrid(t, directory)
	stateDirectory := filepath.Join(directory, "state")
	workspace := filepath.Join(directory, "workspace")
	control := filepath.Join(workspace, "control")
	if err := os.MkdirAll(filepath.Join(workspace, "api"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(control, 0o700); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(workspace, "hook")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nprintf '%s\\n' \"$1\" >> lifecycle-events\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	configuration := `api_version: rungrid/v1
kind: Workspace
project: {name: Tab Only, slug: tab-only, id: tab-only-k7m4q2}
workspace: {root: ..}
terminal: {mode: headless, open: false}
lifecycle:
  before_up:
    - {name: prepare, run: {argv: [./hook, before]}}
  after_down:
    - {name: cleanup, run: {argv: [./hook, after]}}
services:
  - name: api
    source: native
    activation: tab
    working_directory: api
    run: {argv: [sh, -c, "while true; do sleep 1; done"]}
    terminal: {trigger_argv: [make, dev]}
`
	config := filepath.Join(control, ".rungrid.yaml")
	if err := os.WriteFile(config, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	arguments := []string{"--config", config, "--state-dir", stateDirectory}
	for _, action := range [][]string{{"up", "--no-open"}, {"status"}, {"down"}} {
		command := exec.Command(binary, append(arguments, action...)...)
		if output, err := subprocess.Combined(command); err != nil {
			t.Fatalf("rungrid %s: %v\n%s", strings.Join(action, " "), err, output)
		}
	}
	assertE2ELines(t, filepath.Join(workspace, "lifecycle-events"), []string{"before", "after"})
}

func buildRungrid(t *testing.T, directory string) (string, string) {
	t.Helper()
	binary := filepath.Join(directory, "rungrid")
	repositoryRoot := filepath.Clean(filepath.Join("..", "..", ".."))
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = repositoryRoot
	if output, err := subprocess.Combined(build); err != nil {
		t.Fatalf("build Rungrid: %v\n%s", err, output)
	}
	return binary, repositoryRoot
}

func assertE2ELines(t *testing.T, filename string, expected []string) {
	t.Helper()
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	actual := strings.Fields(string(content))
	if strings.Join(actual, ",") != strings.Join(expected, ",") {
		t.Fatalf("lifecycle events = %#v, want %#v", actual, expected)
	}
}

func waitForE2EState(t *testing.T, binary string, baseArguments []string, service, expected string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		command := exec.Command(binary, append(baseArguments, "--json", "status", service)...)
		capture, err := subprocess.Run(command)
		if err == nil {
			var envelope struct {
				Data struct {
					Services []struct {
						Status string `json:"status"`
					} `json:"services"`
				} `json:"data"`
			}
			if json.Unmarshal(capture.Stdout, &envelope) == nil && len(envelope.Data.Services) == 1 && envelope.Data.Services[0].Status == expected {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("service %s did not reach %s", service, expected)
}
