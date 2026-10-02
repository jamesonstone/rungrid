package override

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/state"
)

func TestStoreIsScopedToOneGeneration(t *testing.T) {
	t.Parallel()
	layout := testLayout(t, "example-k7m4q2")
	entries := map[string]Entry{"svc": {Repository: "svc", Path: "/lanes/GH-1", Services: []string{"alpha"}}}
	if err := Save(layout, "generation-a", entries); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(layout, "generation-a")
	if err != nil || loaded["svc"].Path != "/lanes/GH-1" {
		t.Fatalf("Load = %#v, %v", loaded, err)
	}
	stale, err := Load(layout, "generation-b")
	if err != nil || len(stale) != 0 {
		t.Fatalf("another generation inherited overrides: %#v, %v", stale, err)
	}
	info, err := os.Stat(filepath.Join(layout.ProjectDir, storeFileName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("store mode = %v, %v", info, err)
	}
	if err := Remove(layout); err != nil {
		t.Fatal(err)
	}
	if err := Remove(layout); err != nil {
		t.Fatalf("second Remove: %v", err)
	}
	if cleared, _ := Load(layout, "generation-a"); len(cleared) != 0 {
		t.Fatalf("Remove left overrides: %#v", cleared)
	}
}

func TestStoreFailsClosedOnForeignOrCorruptState(t *testing.T) {
	t.Parallel()
	layout := testLayout(t, "example-k7m4q2")
	if err := Save(layout, "generation-a", nil); err != nil {
		t.Fatal(err)
	}
	other := layout
	other.ProjectID = "other-k7m4q2"
	if _, err := Load(other, "generation-a"); errs.Diagnostic(err) != "RG1803" {
		t.Fatalf("foreign project: %v", err)
	}
	filename := filepath.Join(layout.ProjectDir, storeFileName)
	if err := os.WriteFile(filename, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(layout, "generation-a"); errs.Diagnostic(err) != "RG1802" {
		t.Fatalf("corrupt store: %v", err)
	}
	if err := os.Chmod(filename, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(layout, "generation-a"); errs.Diagnostic(err) != "RG1802" {
		t.Fatalf("shared-readable store: %v", err)
	}
}

func testLayout(t *testing.T, projectID string) state.Layout {
	t.Helper()
	layout, err := state.NewLayout(projectID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return layout
}
