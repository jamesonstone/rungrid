package override

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override/overridetest"
)

const testGeneration = "generation-a"

func TestResolveRunsEveryServiceOfAnOverriddenRepositoryFromTheOverride(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	ctx := context.Background()
	recordOverride(t, fixture, "alpha", "GH-1")
	for _, name := range []string{"alpha", "beta"} {
		service, _ := manifest.FindService(&fixture.Loaded.Manifest, name)
		execution, err := Resolve(ctx, fixture.Layout, testGeneration, fixture.Loaded, service, nil)
		if err != nil {
			t.Fatal(err)
		}
		if execution.Override == nil || execution.WorkingDirectory != fixture.Lane || execution.RepositoryRoot != fixture.Lane || execution.Repository != "svc" {
			t.Fatalf("%s execution = %#v", name, execution.Roots)
		}
	}
	alpha, _ := manifest.FindService(&fixture.Loaded.Manifest, "alpha")
	execution, _ := Resolve(ctx, fixture.Layout, testGeneration, fixture.Loaded, alpha, nil)
	got := execution.Argv(alpha.Run.Argv)
	want := []string{"sh", filepath.Join(fixture.Workspace, "tools", "run.sh"), "alpha", "--config=" + filepath.Join(fixture.Lane, "config.yaml"), "./local.sh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("argv = %v\nwant %v", got, want)
	}
}

func TestResolveLeavesUnrelatedExternalAndOtherGenerationsAlone(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	ctx := context.Background()
	recordOverride(t, fixture, "svc", "GH-1")
	gamma, _ := manifest.FindService(&fixture.Loaded.Manifest, "gamma")
	execution, err := Resolve(ctx, fixture.Layout, testGeneration, fixture.Loaded, gamma, nil)
	if err != nil || execution.Override != nil || execution.WorkingDirectory != fixture.Other {
		t.Fatalf("unrelated service = %#v, %v", execution.Roots, err)
	}
	if got := execution.Argv(gamma.Run.Argv); !reflect.DeepEqual(got, gamma.Run.Argv) {
		t.Fatalf("unrelated argv changed: %v", got)
	}
	db, _ := manifest.FindService(&fixture.Loaded.Manifest, "db")
	if external, err := Resolve(ctx, fixture.Layout, testGeneration, fixture.Loaded, db, nil); err != nil || external.Override != nil {
		t.Fatalf("external service was overridden: %#v, %v", external, err)
	}
	alpha, _ := manifest.FindService(&fixture.Loaded.Manifest, "alpha")
	stale, err := Resolve(ctx, fixture.Layout, "generation-b", fixture.Loaded, alpha, nil)
	if err != nil || stale.Override != nil || stale.WorkingDirectory != fixture.Service {
		t.Fatalf("another generation used the override: %#v, %v", stale.Roots, err)
	}
}

func TestResolveOverridesOnlyTheServicesTheEntryNames(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	entry := recordOverride(t, fixture, "svc", "GH-1")
	entry.Services = []string{"alpha"}
	if err := Save(fixture.Layout, testGeneration, map[string]Entry{entry.Repository: entry}); err != nil {
		t.Fatal(err)
	}
	beta, _ := manifest.FindService(&fixture.Loaded.Manifest, "beta")
	execution, err := Resolve(context.Background(), fixture.Layout, testGeneration, fixture.Loaded, beta, nil)
	if err != nil || execution.Override != nil || execution.WorkingDirectory != fixture.Service {
		t.Fatalf("a service outside the entry was overridden: %#v, %v", execution.Roots, err)
	}
}

func TestResolveFailsClosedWhenTheOverrideCheckoutDisappears(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	recordOverride(t, fixture, "svc", "GH-1")
	if err := os.Remove(filepath.Join(fixture.Lane, ".git")); err != nil {
		t.Fatal(err)
	}
	alpha, _ := manifest.FindService(&fixture.Loaded.Manifest, "alpha")
	_, err := Resolve(context.Background(), fixture.Layout, testGeneration, fixture.Loaded, alpha, nil)
	if errs.Diagnostic(err) != "RG1821" {
		t.Fatalf("err = %v", err)
	}
}

func recordOverride(t *testing.T, fixture overridetest.Fixture, target, selector string) Entry {
	t.Helper()
	planned, err := Plan(context.Background(), fixture.Loaded, Request{Target: target, Selector: selector, Source: SourceCLI}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(fixture.Layout, testGeneration, map[string]Entry{planned.Entry.Repository: planned.Entry}); err != nil {
		t.Fatal(err)
	}
	return planned.Entry
}
