package update

import (
	"github.com/jamesonstone/rungrid/internal/maintenance"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestFirstRunInstallsAndSecondRunIsNoOp(t *testing.T) {
	f := newFixture(t)
	first := f.run(Options{}).Repositories[0]
	if first.Status != StatusUpdated {
		t.Fatalf("first run status = %s (%s)", first.Status, first.Reason)
	}
	commands := append([]string(nil), f.exec.commands...)
	sort.Strings(commands)
	if want := []string{"backend-migrate", "go-mod-download", "npm-ci"}; !reflect.DeepEqual(commands, want) {
		t.Fatalf("first run commands = %v, want %v", commands, want)
	}
	if !reflect.DeepEqual(f.coord.paused, []string{"frontend", "backend"}) || !reflect.DeepEqual(f.coord.restarted, []string{"backend", "frontend"}) {
		t.Fatalf("first run paused %v resumed %v", f.coord.paused, f.coord.resumed)
	}

	second := f.run(Options{}).Repositories[0]
	if second.Status != StatusCurrent || second.Git != "none" {
		t.Fatalf("second run = %s/%s (%s)", second.Status, second.Git, second.Reason)
	}
	if len(f.exec.commands) != 0 || len(f.coord.paused) != 0 {
		t.Fatalf("second run ran %v and paused %v", f.exec.commands, f.coord.paused)
	}
	for _, step := range second.Steps {
		if step.Status != StatusCurrent {
			t.Fatalf("step %s = %s on a no-op run", step.ID, step.Status)
		}
	}
	for _, service := range second.Services {
		if service.Status != StatusCurrent {
			t.Fatalf("service %s = %s on a no-op run", service.Name, service.Status)
		}
	}
}

func TestOnlyChangedInputsRunAndRestart(t *testing.T) {
	f := newFixture(t)
	if result := f.run(Options{Baseline: true}).Repositories[0]; result.Status != StatusCurrent || len(f.exec.commands) != 0 {
		t.Fatalf("baseline = %+v, commands %v", result, f.exec.commands)
	}
	cases := []struct {
		file, content string
		commands      []string
		paused        []string
		frontend      string
		backend       string
	}{
		{"frontend/app.tsx", "export const a = 1\n", nil, nil, StatusSkipped, StatusCurrent},
		{"frontend/package-lock.json", `{"lockfileVersion":3}` + "\n", []string{"npm-ci"}, []string{"frontend"}, StatusUpdated, StatusCurrent},
		{"backend/main.go", "package main\n\nfunc main() {}\n", nil, []string{"backend"}, StatusCurrent, StatusUpdated},
		{"backend/go.sum", "example.com/x v1.0.0 h1:x\n", []string{"go-mod-download"}, []string{"backend"}, StatusCurrent, StatusUpdated},
		{"backend/migrations/002.sql", "create table two();\n", []string{"backend-migrate"}, []string{"backend"}, StatusCurrent, StatusUpdated},
	}
	for _, tc := range cases {
		f.push(tc.file, tc.content)
		result := f.run(Options{}).Repositories[0]
		if result.Status != StatusUpdated || result.Git != "fast-forwarded" || f.head() != result.ToOID {
			t.Fatalf("%s: repository = %s/%s head %s to %s", tc.file, result.Status, result.Git, f.head(), result.ToOID)
		}
		if !reflect.DeepEqual(f.exec.commands, tc.commands) || !reflect.DeepEqual(f.coord.paused, tc.paused) {
			t.Fatalf("%s: commands %v paused %v, want %v and %v", tc.file, f.exec.commands, f.coord.paused, tc.commands, tc.paused)
		}
		if got := serviceStatus(t, result, "frontend").Status; got != tc.frontend {
			t.Fatalf("%s: frontend = %s, want %s", tc.file, got, tc.frontend)
		}
		if got := serviceStatus(t, result, "backend").Status; got != tc.backend {
			t.Fatalf("%s: backend = %s, want %s", tc.file, got, tc.backend)
		}
		if again := f.run(Options{}).Repositories[0]; again.Status != StatusCurrent || len(f.exec.commands)+len(f.coord.paused) != 0 {
			t.Fatalf("%s: repeated run was not a no-op: %+v", tc.file, again)
		}
	}
}

func TestDirtyCheckoutIsSkippedUntouched(t *testing.T) {
	f := newFixture(t)
	f.push("backend/main.go", "package main\n// remote\n")
	before := f.head()
	writeFile(t, filepath.Join(f.primary, "backend", "main.go"), "package main\n// local\n")
	result := f.run(Options{}).Repositories[0]
	if result.Status != StatusSkipped || result.Reason == "" || f.head() != before {
		t.Fatalf("dirty checkout = %s (%s), head moved %v", result.Status, result.Reason, f.head() != before)
	}
	if len(f.exec.commands)+len(f.coord.paused) != 0 {
		t.Fatalf("dirty checkout ran %v paused %v", f.exec.commands, f.coord.paused)
	}
}

func TestDryRunChangesNothing(t *testing.T) {
	f := newFixture(t)
	f.run(Options{Baseline: true})
	statePath := filepath.Join(f.layout.ProjectDir, storePath)
	recorded, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	f.push("frontend/package-lock.json", `{"lockfileVersion":3}`+"\n")
	gitRun(t, f.primary, "fetch", "origin")
	before := f.head()
	result := f.run(Options{DryRun: true}).Repositories[0]
	if result.Status != StatusUpdated || result.Git != "would-fast-forward" || f.head() != before {
		t.Fatalf("dry run = %s/%s, head moved %v", result.Status, result.Git, f.head() != before)
	}
	if len(f.exec.commands)+len(f.coord.paused) != 0 {
		t.Fatalf("dry run ran %v paused %v", f.exec.commands, f.coord.paused)
	}
	if got := serviceStatus(t, result, "frontend"); got.Status != StatusUpdated {
		t.Fatalf("dry run frontend plan = %+v", got)
	}
	if after, _ := os.ReadFile(statePath); string(after) != string(recorded) {
		t.Fatal("dry run rewrote the input state")
	}
}

func TestFailedStepResumesServicesAndRetries(t *testing.T) {
	f := newFixture(t)
	f.run(Options{Baseline: true})
	f.push("frontend/package-lock.json", `{"lockfileVersion":3}`+"\n")
	f.exec.fail = "npm-ci"
	failed := f.run(Options{}).Repositories[0]
	if failed.Status != StatusFailed || !f.coord.resumed || !reflect.DeepEqual(f.coord.paused, []string{"frontend"}) {
		t.Fatalf("failure = %s resumed %v paused %v", failed.Status, f.coord.resumed, f.coord.paused)
	}
	f.exec.fail = ""
	retried := f.run(Options{}).Repositories[0]
	if retried.Status != StatusUpdated || !reflect.DeepEqual(f.exec.commands, []string{"npm-ci"}) {
		t.Fatalf("retry = %s commands %v", retried.Status, f.exec.commands)
	}
	if again := f.run(Options{}).Repositories[0]; again.Status != StatusCurrent {
		t.Fatalf("after retry = %s", again.Status)
	}
}

func TestOneRepositoryFailureDoesNotStopOthers(t *testing.T) {
	f := newFixture(t)
	f.run(Options{Baseline: true})
	f.push("backend/main.go", "package main\n// next\n")
	broken := f.repository()
	broken.Name, broken.Remote = "broken", "missing-remote"
	report, err := runRepositories(t.Context(), append([]maintenance.Repository{broken}, f.repository()), nil, 0, Options{},
		Dependencies{Layout: f.layout, Coordinator: f.coord, Executor: f.exec})
	if err == nil || len(report.Repositories) != 2 {
		t.Fatalf("expected partial failure across two repositories, got %v", err)
	}
	if report.Repositories[0].Status != StatusFailed || report.Repositories[1].Status != StatusUpdated {
		t.Fatalf("statuses = %s, %s", report.Repositories[0].Status, report.Repositories[1].Status)
	}
}
