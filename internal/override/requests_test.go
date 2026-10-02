package override

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/errs"
	"github.com/jamesonstone/rungrid/internal/override/overridetest"
)

func TestParseFlag(t *testing.T) {
	t.Parallel()
	request, err := ParseFlag(" api = ~/worktrees/acme/api/GH-1 ")
	if err != nil || request.Target != "api" || request.Selector != "~/worktrees/acme/api/GH-1" || request.Source != SourceUpFlag {
		t.Fatalf("ParseFlag = %#v, %v", request, err)
	}
	for _, value := range []string{"api", "=GH-1", "api=", ""} {
		if _, err := ParseFlag(value); errs.Diagnostic(err) != "RG1817" {
			t.Errorf("ParseFlag(%q) = %v", value, err)
		}
	}
}

func TestDeclarationsAreExtractedFromTheGenerationIdentity(t *testing.T) {
	t.Parallel()
	declared := strings.Replace(overridetest.Manifest, "    working_directory: other\n", "    working_directory: other\n    worktree: primary\n", 1)
	plain := overridetest.New(t)
	fixture := overridetest.NewWithManifest(t, declared)
	if fixture.Loaded.WorktreeDeclarations["gamma"] != "primary" {
		t.Fatalf("declarations = %#v", fixture.Loaded.WorktreeDeclarations)
	}
	if strings.Contains(string(fixture.Loaded.MergedYAML), "worktree") || string(fixture.Loaded.MergedYAML) != string(plain.Loaded.MergedYAML) {
		t.Fatal("worktree declaration leaked into the normalized manifest")
	}
	requests := Declared(fixture.Loaded)
	if len(requests) != 1 || requests[0].Target != "gamma" || requests[0].Source != SourceManifest {
		t.Fatalf("Declared = %#v", requests)
	}
}

func TestPlanAllLetsFlagsWinAndRefusesConflictingDeclarations(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	ctx := context.Background()
	planned, err := PlanAll(ctx, fixture.Loaded, []Request{
		{Target: "alpha", Selector: "primary", Source: SourceManifest},
		{Target: "svc", Selector: "GH-1", Source: SourceUpFlag},
	}, nil)
	if err != nil || len(planned) != 1 || planned[0].Clear || planned[0].Entry.Source != SourceUpFlag {
		t.Fatalf("PlanAll = %#v, %v", planned, err)
	}
	if entries := Entries(planned); len(entries) != 1 || entries[0].Path != fixture.Lane {
		t.Fatalf("Entries = %#v", entries)
	}
	_, err = PlanAll(ctx, fixture.Loaded, []Request{
		{Target: "alpha", Selector: "primary", Source: SourceManifest},
		{Target: "beta", Selector: "GH-1", Source: SourceManifest},
	}, nil)
	if errs.Diagnostic(err) != "RG1816" {
		t.Fatalf("conflicting declarations: %v", err)
	}
}

func TestCandidatesListNewestFirstAndMarkTheCurrentCheckout(t *testing.T) {
	t.Parallel()
	fixture := overridetest.New(t)
	ctx := context.Background()
	newer := filepath.Join(fixture.Root, "lanes", "GH-9")
	overridetest.Git(t, fixture.Service, "worktree", "add", "-q", "-b", "GH-9", newer)
	future := time.Now().Add(time.Hour)
	reflog := filepath.Join(fixture.Service, ".git", "worktrees", "GH-9", "logs", "HEAD")
	if err := os.Chtimes(reflog, future, future); err != nil {
		t.Fatal(err)
	}
	overridetest.WriteFile(t, filepath.Join(fixture.Lane, "VERSION"), "dirty\n")
	recordOverride(t, fixture, "svc", "GH-1")
	repository, candidates, err := Candidates(ctx, fixture.Loaded, fixture.Layout, testGeneration, "beta", nil)
	if err != nil || repository.Name != "svc" || len(candidates) != 3 {
		t.Fatalf("Candidates = %#v, %#v, %v", repository, candidates, err)
	}
	if candidates[0].Path != newer || candidates[0].Branch != "GH-9" {
		t.Fatalf("newest candidate = %#v", candidates[0])
	}
	for _, candidate := range candidates {
		if candidate.CreatedAt.IsZero() || candidate.UpdatedAt.IsZero() || candidate.Subject != "initial primary" {
			t.Errorf("candidate metadata missing: %#v", candidate)
		}
		if (candidate.Path == fixture.Lane) != candidate.Current || (candidate.Path == fixture.Lane) != candidate.Dirty {
			t.Errorf("current/dirty wrong: %#v", candidate)
		}
		if (candidate.Path == fixture.Service) != candidate.Primary {
			t.Errorf("primary wrong: %#v", candidate)
		}
	}
}
