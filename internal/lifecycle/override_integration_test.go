//go:build darwin || linux

package lifecycle

import (
	"context"
	"testing"

	"github.com/jamesonstone/rungrid/internal/override"
	"github.com/jamesonstone/rungrid/internal/override/overridetest"
)

// TestOverrideLifecycleAcrossClearAndResume drives the complete override
// lifecycle against real Git checkouts: two services sharing one repository
// move to a worktree together, an unrelated service keeps its process, clear
// restores the original checkout, and a resume after the terminal restarts
// keeps the override, while down ends it.
func TestOverrideLifecycleAcrossClearAndResume(t *testing.T) {
	fixture := overridetest.New(t)
	active := overrideActive(t, fixture)
	runtime := newFakeRuntime(active, "alpha", "beta", "gamma")
	gammaPID := runtime.pids["gamma"]

	report := applyFake(t, active, runtime, OverrideChange{Operation: "set", Set: []override.Planned{planOverride(t, fixture, "alpha", "GH-1")}})
	if len(report.Services) != 2 || report.Services[0].Action != override.ActionRestarted || report.Services[1].Action != override.ActionRestarted {
		t.Fatalf("set services = %#v", report.Services)
	}
	for _, name := range []string{"alpha", "beta"} {
		if runtime.cwd[name] != fixture.Lane {
			t.Fatalf("%s restarted in %q, want %q", name, runtime.cwd[name], fixture.Lane)
		}
	}
	if runtime.pids["gamma"] != gammaPID || len(runtime.stops) != 2 {
		t.Fatalf("unrelated service was touched: pid %d, stops %v", runtime.pids["gamma"], runtime.stops)
	}

	applyFake(t, active, runtime, OverrideChange{Operation: "clear", Clear: []string{"svc"}})
	if runtime.cwd["alpha"] != fixture.Service || runtime.cwd["beta"] != fixture.Service {
		t.Fatalf("clear did not restore the original checkout: %#v", runtime.cwd)
	}

	applyFake(t, active, runtime, OverrideChange{Operation: "set", Set: []override.Planned{planOverride(t, fixture, "svc", "GH-1")}})
	// Simulate a terminal restart: alpha died with its window, beta was
	// stopped on purpose, and a new process reloads state from disk.
	runtime.statuses["alpha"], runtime.statuses["beta"] = "Completed", "Completed"
	if err := recordStopIntent(active.Layout, overrideGeneration, "beta"); err != nil {
		t.Fatal(err)
	}
	if err := seedOverrides(active.Layout, overrideGeneration, UpOptions{PreserveOverrides: true}); err != nil {
		t.Fatal(err)
	}
	resumed := overrideActive(t, fixture)
	runtime.active = resumed
	for _, name := range []string{"alpha", "beta", "gamma"} {
		action := planServiceResume(resumeServiceInput{
			Source: "native", Activation: "workspace", Status: runtime.statuses[name],
			StopIntended: stopIntended(resumed.Layout, overrideGeneration, name),
		})
		if action == ResumeRestart {
			if err := runtime.start(context.Background(), name); err != nil {
				t.Fatal(err)
			}
		}
	}
	if runtime.cwd["alpha"] != fixture.Lane || runtime.statuses["beta"] != "Completed" || runtime.pids["gamma"] != gammaPID {
		t.Fatalf("resume: cwd %#v statuses %#v gamma pid %d", runtime.cwd, runtime.statuses, runtime.pids["gamma"])
	}

	if err := DownProject(context.Background(), fixture.Layout); err != nil {
		t.Fatal(err)
	}
	if entries, _ := override.Load(fixture.Layout, overrideGeneration); len(entries) != 0 {
		t.Fatalf("down kept overrides: %#v", entries)
	}
}
