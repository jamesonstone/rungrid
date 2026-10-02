//go:build darwin || linux

package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/override/overridetest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

const overrideGeneration = "generation-a"

// fakeRuntime stands in for Process Compose. Starting a service resolves its
// checkout exactly as `rungrid internal exec` does and records where it ran.
type fakeRuntime struct {
	active   Active
	statuses map[string]string
	pids     map[string]int
	cwd      map[string]string
	stops    []string
	starts   []string
	nextPID  int
	startErr error
}

func newFakeRuntime(active Active, running ...string) *fakeRuntime {
	runtime := &fakeRuntime{active: active, statuses: map[string]string{}, pids: map[string]int{}, cwd: map[string]string{}, nextPID: 100}
	for _, name := range running {
		runtime.statuses[name] = "Running"
		runtime.nextPID++
		runtime.pids[name] = runtime.nextPID
	}
	return runtime
}

func (f *fakeRuntime) Get(_ context.Context, name string) (processcompose.ProcessState, error) {
	status, exists := f.statuses[name]
	if !exists {
		return processcompose.ProcessState{}, errors.New("not found")
	}
	return processcompose.ProcessState{Name: name, Status: status, PID: f.pids[name]}, nil
}

func (f *fakeRuntime) Stop(_ context.Context, name string) error {
	f.stops = append(f.stops, name)
	f.statuses[name], f.pids[name] = "Completed", 0
	return nil
}

func (f *fakeRuntime) start(ctx context.Context, name string) error {
	if f.startErr != nil {
		return f.startErr
	}
	service, _ := manifest.FindService(f.active.Manifest, name)
	loaded := &manifest.Loaded{Manifest: *f.active.Manifest, WorkspaceRoot: f.active.Runtime.WorkspaceRoot}
	execution, err := override.Resolve(ctx, f.active.Layout, f.active.Runtime.GenerationID, loaded, service, nil)
	if err != nil {
		return err
	}
	f.starts = append(f.starts, name)
	f.nextPID++
	f.statuses[name], f.pids[name], f.cwd[name] = "Running", f.nextPID, execution.WorkingDirectory
	return nil
}

func overrideActive(t *testing.T, fixture overridetest.Fixture) Active {
	t.Helper()
	return Active{
		Layout:   fixture.Layout,
		Runtime:  supervisor.Runtime{GenerationID: overrideGeneration, WorkspaceRoot: fixture.Workspace},
		Manifest: &fixture.Loaded.Manifest,
	}
}

func planOverride(t *testing.T, fixture overridetest.Fixture, target, selector string) override.Planned {
	t.Helper()
	planned, err := override.Plan(context.Background(), fixture.Loaded, override.Request{Target: target, Selector: selector, Source: override.SourceCLI}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return planned
}

// applyFake mirrors ApplyOverrides with the fake runtime in place of the
// verified Process Compose client.
func applyFake(t *testing.T, active Active, runtime *fakeRuntime, change OverrideChange) override.Report {
	t.Helper()
	report, affected, err := recordOverrideChange(active, change)
	if err != nil {
		t.Fatal(err)
	}
	report.Services = restartAffected(context.Background(), active, runtime, runtime.start, affected)
	return report
}

func TestRecordOverrideChangeSetsClearsAndSyncsExclusively(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	active := overrideActive(t, fixture)
	otherLane := fixture.Root + "/lanes/other-GH-4"
	overridetest.Git(t, fixture.Other, "worktree", "add", "-q", "-b", "GH-4", otherLane)
	report, affected, err := recordOverrideChange(active, OverrideChange{Operation: "set", Set: []override.Planned{planOverride(t, fixture, "svc", "GH-1"), planOverride(t, fixture, "gamma", "GH-4")}})
	if err != nil || len(report.Set) != 2 || len(affected) != 3 {
		t.Fatalf("set: %#v, %v, %v", report, affected, err)
	}
	_, affected, err = recordOverrideChange(active, OverrideChange{Operation: "set", Set: []override.Planned{planOverride(t, fixture, "svc", "GH-1")}})
	if err != nil || len(affected) != 0 {
		t.Fatalf("re-setting the same checkout restarted services: %v, %v", affected, err)
	}
	report, affected, err = recordOverrideChange(active, OverrideChange{Operation: "sync", Set: []override.Planned{planOverride(t, fixture, "svc", "GH-1")}, Exclusive: true})
	if err != nil || len(report.Cleared) != 1 || report.Cleared[0].Repository != "other" || affected["gamma"] != "other" || len(affected) != 1 {
		t.Fatalf("exclusive sync: %#v, %v, %v", report, affected, err)
	}
	report, _, err = recordOverrideChange(active, OverrideChange{Operation: "clear", Clear: []string{"other"}})
	if err != nil || len(report.Cleared) != 0 || len(report.Warnings) != 1 {
		t.Fatalf("clearing an absent override: %#v, %v", report, err)
	}
	report, _, err = recordOverrideChange(active, OverrideChange{Operation: "clear", Exclusive: true})
	if err != nil || len(report.Cleared) != 1 {
		t.Fatalf("clear all: %#v, %v", report, err)
	}
	if entries, _ := override.Load(active.Layout, overrideGeneration); len(entries) != 0 {
		t.Fatalf("clear all left %#v", entries)
	}
}

func TestRestartAffectedHonorsActivationAndStopIntent(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	configuration := fixture.Loaded.Manifest
	configuration.Services = append([]manifest.Service(nil), configuration.Services...)
	configuration.Services[1].Activation = "tab"
	configuration.Services[1].Terminal.TriggerArgv = []string{"make", "dev"}
	active := overrideActive(t, fixture)
	active.Manifest = &configuration
	runtime := newFakeRuntime(active, "beta")
	runtime.statuses["alpha"] = "Completed"
	if err := recordStopIntent(active.Layout, overrideGeneration, "alpha"); err != nil {
		t.Fatal(err)
	}
	services := applyFake(t, active, runtime, OverrideChange{Operation: "set", Set: []override.Planned{planOverride(t, fixture, "svc", "GH-1")}}).Services
	if len(services) != 2 || services[0].Action != override.ActionStopped || services[1].Action != override.ActionTabStopped {
		t.Fatalf("services = %#v", services)
	}
	if len(runtime.starts) != 0 || len(runtime.stops) != 1 || runtime.stops[0] != "beta" {
		t.Fatalf("starts %v stops %v", runtime.starts, runtime.stops)
	}
	if !stopIntended(active.Layout, overrideGeneration, "alpha") {
		t.Fatal("the operator stop intent was cleared")
	}
	delete(runtime.statuses, "beta")
	services = applyFake(t, active, runtime, OverrideChange{Operation: "clear", Clear: []string{"svc"}}).Services
	if services[1].Action != override.ActionRestartFailed {
		t.Fatalf("an unreadable service state was not reported as a failure: %#v", services[1])
	}
	applyFake(t, active, runtime, OverrideChange{Operation: "set", Set: []override.Planned{planOverride(t, fixture, "svc", "GH-1")}})
	runtime.statuses["beta"] = "Completed"
	runtime.startErr = errors.New("boom")
	runtime.statuses["alpha"] = "Running"
	services = applyFake(t, active, runtime, OverrideChange{Operation: "clear", Clear: []string{"svc"}}).Services
	if services[0].Action != override.ActionRestartFailed || services[1].Action != override.ActionTabIdle {
		t.Fatalf("services after clear = %#v", services)
	}
}

func TestApplyOverridesRefusesAChangedRuntime(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	active := overrideActive(t, fixture)
	_, err := ApplyOverrides(context.Background(), active, OverrideChange{Operation: "set", Set: []override.Planned{planOverride(t, fixture, "svc", "GH-1")}})
	if errs.Diagnostic(err) != "RG1824" {
		t.Fatalf("err = %v", err)
	}
	if entries, _ := override.Load(active.Layout, overrideGeneration); len(entries) != 0 {
		t.Fatalf("a refused change was recorded: %#v", entries)
	}
}

func TestSeedOverridesStartsCleanOrPreservesForRecovery(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	planned := planOverride(t, fixture, "svc", "GH-1")
	if err := override.Save(fixture.Layout, overrideGeneration, map[string]override.Entry{"svc": planned.Entry}); err != nil {
		t.Fatal(err)
	}
	if err := seedOverrides(fixture.Layout, overrideGeneration, UpOptions{PreserveOverrides: true}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := override.Load(fixture.Layout, overrideGeneration); len(entries) != 1 {
		t.Fatalf("recovery dropped overrides: %#v", entries)
	}
	if err := seedOverrides(fixture.Layout, overrideGeneration, UpOptions{}); err != nil {
		t.Fatal(err)
	}
	if entries, _ := override.Load(fixture.Layout, overrideGeneration); len(entries) != 0 {
		t.Fatalf("a fresh up kept leftover overrides: %#v", entries)
	}
	if err := seedOverrides(fixture.Layout, overrideGeneration, UpOptions{Overrides: []override.Entry{planned.Entry}}); err != nil {
		t.Fatal(err)
	}
	entries, _ := override.Load(fixture.Layout, overrideGeneration)
	if entries["svc"].Path != fixture.Lane || entries["svc"].SetAt == "" {
		t.Fatalf("up --override was not seeded: %#v", entries)
	}
}
