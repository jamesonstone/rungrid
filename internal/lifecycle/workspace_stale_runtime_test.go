//go:build darwin || linux

package lifecycle

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/planner"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/workspace"
)

func TestReconcileBeforeUpCompletesCleanupAfterRuntimeProcessDies(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeExecutable(t, filepath.Join(root, "hook"), "#!/bin/sh\nprintf 'after\\n' >> lifecycle-events\n")
	layout, journal := lifecycleFixture(t, root, nil, []manifest.LifecycleCommand{
		lifecycleCommand("cleanup", "./hook"),
	})
	runtimeState := supervisor.Runtime{
		APIVersion: "rungrid/output/v1", ProjectID: layout.ProjectID,
		GenerationID: journal.GenerationID, PID: 1 << 30,
		ProcessIdentity: "Mon Jan  1 00:00:00 2024",
		ProcessCommand:  "process-compose -U -u ../../runtime.sock",
		Socket:          filepath.Join(layout.ProjectDir, "runtime.sock"),
		SocketDevice:    1, SocketInode: 2,
	}
	completeRuntimeScope(t, layout, journal.GenerationID, &runtimeState)
	if err := supervisor.Write(layout, runtimeState); err != nil {
		t.Fatal(err)
	}
	journal.State = workspace.StateActive
	attachRuntime(&journal, runtimeState)
	if err := workspace.WriteJournal(layout, journal); err != nil {
		t.Fatal(err)
	}
	generatedManifest, err := manifest.LoadGenerated(
		filepath.Join(layout.ProjectDir, "generations", journal.GenerationID, "manifest.yaml"),
		journal.WorkspaceRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	loaded := &manifest.Loaded{Manifest: *generatedManifest, WorkspaceRoot: journal.WorkspaceRoot}
	planned := planner.Plan{
		GenerationID:    journal.GenerationID,
		ManifestSHA256:  journal.ManifestSHA256,
		LifecycleSHA256: journal.LifecycleSHA256,
	}

	_, reused, err := reconcileBeforeUp(context.Background(), layout, loaded, planned)
	if err != nil {
		t.Fatal(err)
	}
	if reused {
		t.Fatal("dead runtime was reused")
	}
	assertLines(t, filepath.Join(root, "lifecycle-events"), []string{"after"})
	if _, err := os.Lstat(filepath.Join(layout.ProjectDir, "runtime.json")); !os.IsNotExist(err) {
		t.Fatalf("stale runtime record remains: %v", err)
	}
	journal, err = workspace.ReadJournal(layout)
	if err != nil {
		t.Fatal(err)
	}
	if journal.State != workspace.StateInactive || journal.TeardownRequired || journal.Runtime != nil {
		t.Fatalf("cleanup did not retire the runtime identity: %#v", journal)
	}
}

func TestReconcileBeforeUpPreservesDeadRuntimeWithoutJournalIdentity(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	layout, journal := lifecycleFixture(t, root, nil, nil)
	runtimeState := supervisor.Runtime{
		APIVersion: "rungrid/output/v1", ProjectID: layout.ProjectID,
		GenerationID: journal.GenerationID, PID: 1 << 30,
		ProcessIdentity: "Mon Jan  1 00:00:00 2024",
		ProcessCommand:  "process-compose -U -u ../../runtime.sock",
		Socket:          filepath.Join(layout.ProjectDir, "runtime.sock"),
		SocketDevice:    1, SocketInode: 2,
	}
	completeRuntimeScope(t, layout, journal.GenerationID, &runtimeState)
	if err := supervisor.Write(layout, runtimeState); err != nil {
		t.Fatal(err)
	}
	journal.State = workspace.StateActive
	if err := workspace.WriteJournal(layout, journal); err != nil {
		t.Fatal(err)
	}
	generatedManifest, err := manifest.LoadGenerated(
		filepath.Join(layout.ProjectDir, "generations", journal.GenerationID, "manifest.yaml"),
		journal.WorkspaceRoot,
	)
	if err != nil {
		t.Fatal(err)
	}
	loaded := &manifest.Loaded{Manifest: *generatedManifest, WorkspaceRoot: journal.WorkspaceRoot}
	planned := planner.Plan{
		GenerationID:    journal.GenerationID,
		ManifestSHA256:  journal.ManifestSHA256,
		LifecycleSHA256: journal.LifecycleSHA256,
	}

	_, _, err = reconcileBeforeUp(context.Background(), layout, loaded, planned)
	if err == nil || !strings.Contains(err.Error(), "runtime PID is stale") {
		t.Fatalf("expected stale runtime conflict, got %v", err)
	}
	if _, err := os.Lstat(filepath.Join(layout.ProjectDir, "runtime.json")); err != nil {
		t.Fatalf("ambiguous runtime record was not preserved: %v", err)
	}
}
