package checkout

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUseSelectsOneServiceWorktree(t *testing.T) {
	t.Parallel()
	fixture := newRepositoryFixture(t)
	loaded, layout := loadedServices(t, fixture)
	report, err := Use(context.Background(), layout, loaded, "api", "GH-12", nil)
	if err != nil {
		t.Fatal(err)
	}
	lane := mustPhysical(t, fixture.lane)
	if report.Action != "selected" || report.Path != lane || report.Branch != "GH-12" {
		t.Fatalf("unexpected use report %#v", report)
	}
	api, err := Resolve(context.Background(), layout, loaded, &loaded.Manifest.Services[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	if api.RepositoryRoot != lane || !api.Selected {
		t.Fatalf("api did not bind to lane: %#v", api)
	}
	worker, err := Resolve(context.Background(), layout, loaded, &loaded.Manifest.Services[1], nil)
	if err != nil {
		t.Fatal(err)
	}
	if worker.Selected || worker.RepositoryRoot == lane {
		t.Fatalf("worker should keep the declared root, got %#v", worker)
	}
}

func TestUsePrimaryAndClear(t *testing.T) {
	t.Parallel()
	fixture := newRepositoryFixture(t)
	loaded, layout := loadedServices(t, fixture)
	if _, err := Use(context.Background(), layout, loaded, "api", filepath.Join(fixture.root, "missing"), nil); err == nil {
		t.Fatal("expected missing path to fail")
	}
	if _, err := Use(context.Background(), layout, loaded, "api", "GH-12", nil); err != nil {
		t.Fatal(err)
	}
	cleared, err := Clear(layout, loaded, "api")
	if err != nil || cleared.Action != "cleared" {
		t.Fatalf("clear failed: %#v %v", cleared, err)
	}
	primaryReport, err := Use(context.Background(), layout, loaded, "api", "primary", nil)
	if err != nil {
		t.Fatal(err)
	}
	if primaryReport.Path != mustPhysical(t, fixture.primary) {
		t.Fatalf("primary selector bound %q", primaryReport.Path)
	}
}

func TestMatchRefusesDetachedAndForeignPaths(t *testing.T) {
	t.Parallel()
	fixture := newRepositoryFixture(t)
	loaded, layout := loadedServices(t, fixture)
	foreign := t.TempDir()
	if _, err := Use(context.Background(), layout, loaded, "api", foreign, nil); err == nil {
		t.Fatal("expected foreign path to fail")
	}
	detached := filepath.Join(fixture.root, "detached")
	gitTest(t, fixture.primary, "worktree", "add", "--detach", detached, "origin/main")
	if _, err := Use(context.Background(), layout, loaded, "api", detached, nil); err == nil {
		t.Fatal("expected detached worktree to fail")
	}
}

func TestResolveUsesSelectedWorktreeWorkingDirectory(t *testing.T) {
	t.Parallel()
	fixture := newRepositoryFixture(t)
	if err := os.Mkdir(filepath.Join(fixture.lane, "svc"), 0o700); err != nil {
		t.Fatal(err)
	}
	loaded, layout := loadedServices(t, fixture)
	loaded.Manifest.Services[0].WorkingDirectory = "svc"
	if _, err := Use(context.Background(), layout, loaded, "api", "GH-12", nil); err != nil {
		t.Fatal(err)
	}
	roots, err := Resolve(context.Background(), layout, loaded, &loaded.Manifest.Services[0], nil)
	if err != nil {
		t.Fatal(err)
	}
	want := mustPhysical(t, filepath.Join(fixture.lane, "svc"))
	if roots.WorkingDirectory != want || roots.RepositoryRoot != mustPhysical(t, fixture.lane) {
		t.Fatalf("selected cwd was %#v, want %q", roots, want)
	}
}
