package reconcile

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/maintenance"
)

func TestLiveCWDProcessBlocksStaleRootSwitch(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof is unavailable")
	}
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	process := exec.Command("sleep", "30")
	process.Dir = fixture.primary
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })

	report, err := Run(context.Background(), Options{
		Target: fixture.primary, Runner: safeRunner{realLsof: true},
		Now: func() time.Time { return time.Now().Add(96 * time.Hour) },
	})
	if err != nil {
		t.Fatal(err)
	}
	root := report.Repositories[0].Root
	if root.Reason != "primary-in-use" || root.Action != "preserved" || len(root.CWDProcessIDs) == 0 {
		t.Fatalf("root = %#v", root)
	}
	if branch := gitFixture(t, fixture.primary, "branch", "--show-current"); branch != "GH-1" {
		t.Fatalf("branch = %s", branch)
	}
}

func TestCoordinatorOwnedProcessCanBePausedForRootSwitch(t *testing.T) {
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	coordinator := &ownedCoordinator{}
	runner := ownedProcessRunner{safeRunner: safeRunner{}, root: fixture.primary}

	report, err := Run(context.Background(), Options{
		Target: fixture.primary, Runner: runner, Coordinator: coordinator,
		Now: func() time.Time { return time.Now().Add(96 * time.Hour) },
	})
	if err != nil {
		t.Fatal(err)
	}
	root := report.Repositories[0].Root
	if root.Action != "switched" || coordinator.pauses != 1 || coordinator.resumes != 1 {
		t.Fatalf("root = %#v, coordinator = %#v", root, coordinator)
	}
	if len(root.OwnedProcessIDs) != 1 || root.OwnedProcessIDs[0] != "42" {
		t.Fatalf("owned process evidence = %#v", root.OwnedProcessIDs)
	}
}

func TestOpenDirtyPathBlocksStaleRootRecovery(t *testing.T) {
	if _, err := exec.LookPath("lsof"); err != nil {
		t.Skip("lsof is unavailable")
	}
	fixture := newReconcileFixture(t)
	gitFixture(t, fixture.primary, "switch", "-c", "GH-1")
	dirtyPath := filepath.Join(fixture.primary, "README.md")
	writeFixture(t, dirtyPath, "unfinished\n")
	process := exec.Command("sh", "-c", `exec 3<"$1"; exec sleep 30`, "sh", dirtyPath)
	process.Dir = fixture.root
	if err := process.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })

	report, err := Run(context.Background(), Options{
		Target: fixture.primary, Runner: safeRunner{realLsof: true},
		Now: func() time.Time { return time.Now().Add(96 * time.Hour) },
	})
	if err != nil {
		t.Fatal(err)
	}
	root := report.Repositories[0].Root
	if root.Reason != "primary-in-use" || len(root.OpenPathProcessIDs) == 0 {
		t.Fatalf("root = %#v", root)
	}
	if status := gitFixture(t, fixture.primary, "status", "--porcelain"); status == "" {
		t.Fatal("dirty work was not preserved")
	}
}

func TestDefaultSyncDoesNotPauseServicesWhenUnownedProcessExists(t *testing.T) {
	coordinator := &ownedCoordinator{}
	guard := processGuardCoordinator{
		delegate: coordinator,
		runner:   lsofOnlyRunner{content: []byte("p99\nn/workspace/repository\n")},
	}
	_, _, err := guard.Pause(context.Background(), "/workspace/repository")
	if err == nil || coordinator.pauses != 0 || coordinator.resumes != 0 {
		t.Fatalf("err = %v, coordinator = %#v", err, coordinator)
	}
}

type ownedProcessRunner struct {
	safeRunner
	root string
}

type lsofOnlyRunner struct{ content []byte }

func (runner lsofOnlyRunner) Run(context.Context, string, string, ...string) ([]byte, error) {
	return runner.content, nil
}

func (runner ownedProcessRunner) Run(ctx context.Context, directory, executable string, arguments ...string) ([]byte, error) {
	if executable == "lsof" {
		return []byte(fmt.Sprintf("p42\nn%s\n", filepath.Clean(runner.root))), nil
	}
	return runner.safeRunner.Run(ctx, directory, executable, arguments...)
}

type ownedCoordinator struct {
	pauses  int
	resumes int
}

func (*ownedCoordinator) AffectedServices(context.Context, string) ([]string, error) {
	return []string{"api"}, nil
}

func (*ownedCoordinator) OwnedProcessIDs(context.Context, string) ([]string, error) {
	return []string{"42"}, nil
}

func (coordinator *ownedCoordinator) Pause(context.Context, string) ([]string, maintenance.ResumeFunc, error) {
	coordinator.pauses++
	return []string{"api"}, func(context.Context) error {
		coordinator.resumes++
		return nil
	}, nil
}
