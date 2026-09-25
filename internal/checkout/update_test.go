package checkout

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUpdateFastForwardsSelectedFeatureWorktree(t *testing.T) {
	t.Parallel()
	fixture := newRepositoryFixture(t)
	loaded, layout := loadedServices(t, fixture)
	if _, err := Use(context.Background(), layout, loaded, "api", "GH-12", nil); err != nil {
		t.Fatal(err)
	}
	remoteOID := fixture.advanceLane(t, "updated")
	dry, err := Update(context.Background(), layout, loaded, UpdateOptions{DryRun: true}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(dry.Targets) != 1 || dry.Targets[0].Action != "would-fast-forward" {
		t.Fatalf("unexpected dry-run %#v", dry)
	}
	headBefore := gitTest(t, fixture.lane, "rev-parse", "HEAD")
	if headBefore == remoteOID {
		t.Fatal("lane was already current")
	}
	applied, err := Update(context.Background(), layout, loaded, UpdateOptions{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Targets) != 1 || applied.Targets[0].Action != "fast-forwarded" {
		t.Fatalf("unexpected apply %#v", applied)
	}
	if gitTest(t, fixture.lane, "rev-parse", "HEAD") != remoteOID {
		t.Fatal("selected worktree did not fast-forward")
	}
}

func TestUpdatePreservesDirtySelectedWorktree(t *testing.T) {
	t.Parallel()
	fixture := newRepositoryFixture(t)
	loaded, layout := loadedServices(t, fixture)
	if _, err := Use(context.Background(), layout, loaded, "api", "GH-12", nil); err != nil {
		t.Fatal(err)
	}
	fixture.advanceLane(t, "remote-change")
	if err := os.WriteFile(filepath.Join(fixture.lane, "README.md"), []byte("local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := Update(context.Background(), layout, loaded, UpdateOptions{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Targets) != 1 || report.Targets[0].State != "dirty" {
		t.Fatalf("expected dirty preservation, got %#v", report)
	}
}
