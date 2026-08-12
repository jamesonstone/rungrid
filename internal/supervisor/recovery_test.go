//go:build darwin || linux

package supervisor

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesonstone/rungrid/internal/state"
)

func TestRetireStaleRuntimeRemovesDeadRecord(t *testing.T) {
	layout, runtimeState := staleRuntimeFixture(t)
	retired, err := RetireStaleRuntime(layout, runtimeState)
	if err != nil {
		t.Fatal(err)
	}
	if !retired {
		t.Fatal("dead runtime record was not retired")
	}
	if _, err := os.Lstat(filepath.Join(layout.ProjectDir, "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("stale runtime record still exists: %v", err)
	}
}

func TestRetireStaleRuntimePreservesLivePID(t *testing.T) {
	layout, runtimeState := staleRuntimeFixture(t)
	runtimeState.PID = os.Getpid()
	if err := Write(layout, runtimeState); err != nil {
		t.Fatal(err)
	}
	retired, err := RetireStaleRuntime(layout, runtimeState)
	if err != nil {
		t.Fatal(err)
	}
	if retired {
		t.Fatal("record with a live PID was retired")
	}
	assertRuntimeRecordExists(t, layout)
}

func TestRetireStaleRuntimePreservesPresentSocketPath(t *testing.T) {
	layout, runtimeState := staleRuntimeFixture(t)
	if err := os.WriteFile(runtimeState.Socket, []byte("occupied\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	retired, err := RetireStaleRuntime(layout, runtimeState)
	if err != nil {
		t.Fatal(err)
	}
	if retired {
		t.Fatal("record with a present socket was retired")
	}
	assertRuntimeRecordExists(t, layout)
}

func staleRuntimeFixture(t *testing.T) (state.Layout, Runtime) {
	t.Helper()
	layout, err := state.NewLayout("example-k7m4q2", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	runtimeState := Runtime{
		APIVersion: "rungrid/output/v1", ProjectID: layout.ProjectID,
		GenerationID: "generation", PID: 1 << 30,
		ProcessIdentity: "Mon Jan  1 00:00:00 2024",
		ProcessCommand:  "process-compose -U -u ../../runtime.sock",
		Socket:          filepath.Join(layout.ProjectDir, "runtime.sock"),
		SocketDevice:    1, SocketInode: 2, SocketOwnerUID: uint32(os.Getuid()),
	}
	generation := filepath.Join(layout.ProjectDir, "generations", runtimeState.GenerationID)
	if err := os.MkdirAll(generation, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestContent := []byte("effective manifest\n")
	configurationContent := []byte("process compose\n")
	if err := os.WriteFile(filepath.Join(generation, "manifest.yaml"), manifestContent, 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeState.Configuration = filepath.Join(generation, "process-compose.yaml")
	if err := os.WriteFile(runtimeState.Configuration, configurationContent, 0o600); err != nil {
		t.Fatal(err)
	}
	runtimeState.EffectiveManifestSHA256 = state.Hash(manifestContent)
	runtimeState.ConfigurationHash = state.Hash(configurationContent)
	if err := state.WriteFileAtomic(layout.ProjectDir, "current", []byte(runtimeState.GenerationID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Write(layout, runtimeState); err != nil {
		t.Fatal(err)
	}
	return layout, runtimeState
}

func assertRuntimeRecordExists(t *testing.T, layout state.Layout) {
	t.Helper()
	if _, err := os.Lstat(filepath.Join(layout.ProjectDir, "runtime.json")); err != nil {
		t.Fatalf("runtime record was not preserved: %v", err)
	}
}
