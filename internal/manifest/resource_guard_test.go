package manifest

import (
	"strings"
	"testing"
	"time"
)

func TestResourceGuardDefaultsAndServiceOverrides(t *testing.T) {
	configuration := validManifest(t)
	configuration.Services[0].ResourceGuard = &ResourceGuard{
		SustainedWindow: Duration{Duration: 90 * time.Second},
		Emergency:       ResourceLimits{CPUPercent: 60},
	}
	configuration.ApplyDefaults()
	runtimePolicy := configuration.Runtime.ResourceGuard
	if runtimePolicy.SampleInterval.Duration != time.Second || runtimePolicy.LearningWindow.Duration != 15*time.Minute {
		t.Fatalf("unexpected runtime defaults: %#v", runtimePolicy)
	}
	servicePolicy := EffectiveServiceResourceGuard(runtimePolicy, &configuration.Services[0])
	if servicePolicy.SustainedWindow.Duration != 90*time.Second || servicePolicy.Emergency.CPUPercent != 60 {
		t.Fatalf("service override was not applied: %#v", servicePolicy)
	}
	if servicePolicy.Emergency.MemoryPercent != 50 || servicePolicy.RestartLimit != 3 {
		t.Fatalf("service override did not inherit runtime values: %#v", servicePolicy)
	}
}

func TestResourceGuardRejectsUnsafeOverrides(t *testing.T) {
	configuration := validManifest(t)
	configuration.ApplyDefaults()
	configuration.Runtime.ResourceGuard.SampleInterval.Duration = 100 * time.Millisecond
	configuration.Runtime.ResourceGuard.Emergency.CPUPercent = 101
	configuration.Runtime.ResourceGuard.BackoffInitial.Duration = 9 * time.Second
	configuration.Runtime.ResourceGuard.BackoffMaximum.Duration = 2 * time.Second
	err := Validate(&configuration, t.TempDir())
	for _, expected := range []string{"sample_interval", "cpu_percent", "backoff_initial"} {
		if err == nil || !strings.Contains(err.Error(), expected) {
			t.Fatalf("expected %s validation failure, got %v", expected, err)
		}
	}
}

func validManifest(t *testing.T) Manifest {
	t.Helper()
	return Manifest{
		APIVersion: APIVersion,
		Kind:       Kind,
		Project:    Project{Name: "Example", Slug: "example", ID: "example-k7m4q2"},
		Workspace:  Workspace{Root: "."},
		Terminal:   Terminal{Mode: "headless", Open: Bool(false)},
		Services: []Service{{
			Name: "api", Source: "native", Activation: "workspace", Repository: WorkspaceRepository,
			WorkingDirectory: ".", Run: &Run{Argv: []string{"api"}},
		}},
	}
}
