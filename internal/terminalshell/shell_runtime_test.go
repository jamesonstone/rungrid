//go:build darwin || linux

package terminalshell

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/procidentity"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func TestWatchRuntimeQuiescesOnMatchingShutdown(t *testing.T) {
	layout, err := state.NewLayout("shell-watch-r4k2m7", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	runtimeState := supervisor.Runtime{GenerationID: "generation"}
	shutdown := make(chan string, 1)
	go watchRuntime(context.Background(), ShellOptions{Layout: layout, Runtime: runtimeState}, shutdown)
	marker := filepath.Join("locks", "down-generation.json")
	if err := state.WriteFileAtomic(layout.ProjectDir, marker, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case reason := <-shutdown:
		if reason != "workspace shutdown began" {
			t.Fatalf("unexpected shutdown reason %q", reason)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("managed shell did not observe the shutdown marker")
	}
}

func TestStopManagedShellWaitsForChild(t *testing.T) {
	command := exec.Command("sleep", "10")
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { result <- command.Wait() }()
	if err := stopManagedShell(command, result, "test shutdown"); err != nil {
		t.Fatal(err)
	}
	if err := command.Process.Signal(syscall.Signal(0)); err == nil {
		t.Fatal("managed child remains live")
	}
}

func TestWaitGenerationReleasedWaitsForLiveTab(t *testing.T) {
	layout, err := state.NewLayout("tab-wait-r4k2m7", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	identity, err := procidentity.Current()
	if err != nil {
		t.Fatal(err)
	}
	registration := TabRegistration{
		APIVersion: "rungrid/output/v1", ProjectID: layout.ProjectID,
		GenerationID: "generation", Service: "api", PID: os.Getpid(),
		ProcessIdentity: identity,
	}
	path := filepath.Join(layout.ProjectDir, "tabs", "generation-api.json")
	if err := writeTabRegistration(path, registration); err != nil {
		t.Fatal(err)
	}
	go func() {
		time.Sleep(50 * time.Millisecond)
		removeTabRegistration(path, registration)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := WaitGenerationReleased(ctx, layout, "generation", []string{"api"}); err != nil {
		t.Fatal(err)
	}
}
