package override

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/override/overridetest"
)

func TestRepositoriesKeyServicesByCheckout(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	repositories := Repositories(context.Background(), fixture.Loaded, nil)
	if len(repositories) != 2 {
		t.Fatalf("repositories = %#v", repositories)
	}
	other, svc := repositories[0], repositories[1]
	if svc.Name != "svc" || svc.TopLevel != fixture.Service || !reflect.DeepEqual(svc.Managed, []string{"alpha", "beta"}) || !reflect.DeepEqual(svc.External, []string{"db"}) {
		t.Fatalf("svc repository = %#v", svc)
	}
	if other.Name != "other" || !reflect.DeepEqual(other.Managed, []string{"gamma"}) {
		t.Fatalf("other repository = %#v", other)
	}
}

func TestPlanServiceTargetSwitchesEveryServiceOfItsRepository(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	planned, err := Plan(context.Background(), fixture.Loaded, Request{Target: "beta", Selector: "GH-1", Source: SourceCLI}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := planned.Entry
	if planned.Clear || entry.Repository != "svc" || entry.Path != fixture.Lane || entry.OriginalPath != fixture.Service || entry.Branch != "GH-1" {
		t.Fatalf("entry = %#v", entry)
	}
	if !reflect.DeepEqual(entry.Services, []string{"alpha", "beta"}) || len(entry.HeadOID) != 40 || entry.Dirty {
		t.Fatalf("entry services/head/dirty = %#v", entry)
	}
	if !containsText(planned.Warnings, "resolves to repository svc") || !containsText(planned.Warnings, "external services keep their own location: db") {
		t.Fatalf("warnings = %v", planned.Warnings)
	}
	want := []Reanchor{
		{Service: "alpha", Field: "run.argv", Original: "../tools/run.sh", Resolved: filepath.Join(fixture.Workspace, "tools", "run.sh")},
		{Service: "alpha", Field: "run.argv", Original: "--config=../svc/config.yaml", Resolved: "--config=" + filepath.Join(fixture.Lane, "config.yaml")},
		{Service: "beta", Field: "run.argv", Original: "../tools/run.sh", Resolved: filepath.Join(fixture.Workspace, "tools", "run.sh")},
	}
	if !reflect.DeepEqual(entry.Reanchored, want) {
		t.Fatalf("reanchored = %#v\nwant %#v", entry.Reanchored, want)
	}
}

func TestPlanSelectorForms(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	ctx := context.Background()
	for _, selector := range []string{"GH-1", fixture.Lane, filepath.Join(fixture.Lane, ".")} {
		planned, err := Plan(ctx, fixture.Loaded, Request{Target: "svc", Selector: selector}, nil)
		if err != nil || planned.Entry.Path != fixture.Lane || planned.Clear {
			t.Fatalf("selector %q: %#v, %v", selector, planned, err)
		}
	}
	for _, selector := range []string{"primary", "PRIMARY", fixture.Service} {
		planned, err := Plan(ctx, fixture.Loaded, Request{Target: "alpha", Selector: selector}, nil)
		if err != nil || !planned.Clear {
			t.Fatalf("selector %q should clear: %#v, %v", selector, planned, err)
		}
	}
	clone, err := Plan(ctx, fixture.Loaded, Request{Target: "svc", Selector: fixture.Clone}, nil)
	if err != nil || clone.Entry.Path != fixture.Clone {
		t.Fatalf("same-origin clone was refused: %#v, %v", clone, err)
	}
}

func TestPlanWarnsWithoutRefusingADirtyCheckout(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	overridetest.WriteFile(t, filepath.Join(fixture.Lane, "VERSION"), "changed\n")
	planned, err := Plan(context.Background(), fixture.Loaded, Request{Target: "svc", Selector: "GH-1"}, nil)
	if err != nil || !planned.Entry.Dirty || !containsText(planned.Warnings, "uncommitted changes") {
		t.Fatalf("dirty checkout: %#v, %v", planned, err)
	}
}

func TestPlanRefusals(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	overridetest.Git(t, fixture.Service, "worktree", "add", "-q", "-b", "lane", filepath.Join(fixture.Root, "lanes", "GH-2"))
	overridetest.Git(t, fixture.Service, "worktree", "add", "-q", "-b", "GH-2", filepath.Join(fixture.Root, "other-lanes", "GH-3"))
	tests := []struct {
		target, selector, code string
	}{
		{"nope", "GH-1", "RG1805"},
		{"db", "GH-1", "RG1806"},
		{"alpha", filepath.Join(fixture.Root, "missing"), "RG1808"},
		{"alpha", fixture.Plain, "RG1809"},
		{"alpha", fixture.Foreign, "RG1810"},
		{"gamma", fixture.Lane, "RG1810"},
		{"alpha", "GH-2", "RG1811"},
		{"alpha", "GH-404", "RG1812"},
		{"alpha", " ", "RG1812"},
	}
	for _, test := range tests {
		_, err := Plan(context.Background(), fixture.Loaded, Request{Target: test.target, Selector: test.selector}, nil)
		if errs.Diagnostic(err) != test.code {
			t.Errorf("Plan(%q, %q) = %v (%s), want %s", test.target, test.selector, err, errs.Diagnostic(err), test.code)
		}
	}
}

func TestPlanRefusesRepositoryWithOnlyExternalServices(t *testing.T) {
	t.Parallel()
	fixture := overridetest.NewWithManifest(t, strings.Replace(overridetest.Manifest, "  - name: gamma\n    source: native\n    activation: workspace\n    working_directory: other\n    run:\n      argv: [sh, ../tools/run.sh, gamma]\n",
		"  - name: gamma\n    source: external\n    working_directory: other\n    external:\n      url: http://127.0.0.1:9/\n", 1))
	_, err := Plan(context.Background(), fixture.Loaded, Request{Target: "other", Selector: "primary"}, nil)
	if errs.Diagnostic(err) != "RG1807" {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanRefusesMissingWorkingDirectoryAndEscapingProviders(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	ctx := context.Background()
	otherLane := filepath.Join(fixture.Root, "lanes", "other-GH-5")
	overridetest.Git(t, fixture.Other, "worktree", "add", "-q", "-b", "GH-5", otherLane)
	overridetest.WriteFile(t, filepath.Join(fixture.Other, "api", "main.go"), "package main\n")
	overridetest.WriteFile(t, filepath.Join(fixture.Root, "shared.env"), "KEY=value\n")
	if err := os.Symlink(filepath.Join(fixture.Root, "shared.env"), filepath.Join(fixture.Lane, "linked.env")); err != nil {
		t.Fatal(err)
	}
	nested := withServices(fixture, func(services []manifest.Service) {
		services[2].WorkingDirectory = "other/api"
	})
	if _, err := Plan(ctx, nested, Request{Target: "gamma", Selector: otherLane}, nil); errs.Diagnostic(err) != "RG1813" {
		t.Fatalf("missing working directory: %v", err)
	}
	for _, provider := range []manifest.EnvironmentProvider{
		{Type: "dotenv", Path: "../shared.env"},
		{Type: "direnv", Directory: ".."},
		{Type: "command", Argv: []string{"../tools/env.sh"}},
		{Type: "dotenv", Path: "missing.env"},
		{Type: "dotenv", Path: "linked.env", Optional: true},
	} {
		escaping := withServices(fixture, func(services []manifest.Service) {
			services[1].Environment.Providers = []manifest.EnvironmentProvider{provider}
		})
		if _, err := Plan(ctx, escaping, Request{Target: "svc", Selector: "GH-1"}, nil); errs.Diagnostic(err) != "RG1814" {
			t.Fatalf("%s provider: %v", provider.Type, err)
		}
	}
	compose := withServices(fixture, func(services []manifest.Service) {
		services[1].Run, services[1].Source = nil, "compose"
		services[1].Compose = &manifest.Compose{File: "../compose.yaml", Service: "beta"}
	})
	if _, err := Plan(ctx, compose, Request{Target: "svc", Selector: "GH-1"}, nil); errs.Diagnostic(err) != "RG1814" {
		t.Fatalf("escaping compose file: %v", err)
	}
	inside := withServices(fixture, func(services []manifest.Service) {
		services[1].Environment.Providers = []manifest.EnvironmentProvider{{Type: "dotenv", Path: ".env", Optional: true}}
	})
	if _, err := Plan(ctx, inside, Request{Target: "svc", Selector: "GH-1"}, nil); err != nil {
		t.Fatalf("provider inside the checkout was refused: %v", err)
	}
}

func withServices(fixture overridetest.Fixture, change func([]manifest.Service)) *manifest.Loaded {
	loaded := *fixture.Loaded
	loaded.Manifest.Services = append([]manifest.Service(nil), fixture.Loaded.Manifest.Services...)
	change(loaded.Manifest.Services)
	return &loaded
}

func containsText(values []string, wanted string) bool {
	for _, value := range values {
		if strings.Contains(value, wanted) {
			return true
		}
	}
	return false
}
