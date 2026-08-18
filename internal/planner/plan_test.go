package planner

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/present"
)

func TestPlanIncludesPortableLifecycleActions(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	control := filepath.Join(root, "control")
	if err := os.MkdirAll(control, 0o700); err != nil {
		t.Fatal(err)
	}
	configuration := `api_version: rungrid/v1
kind: Workspace
project: {name: Example, slug: example, id: example-k7m4q2}
workspace: {root: ..}
terminal: {mode: headless}
lifecycle:
  before_up:
    - name: prepare
      working_directory: control
      timeout: 5s
      run: {argv: [example-tool, prepare, --token, private-value]}
  after_down:
    - name: cleanup
      working_directory: control
      timeout: 6s
      run: {argv: [example-tool, cleanup]}
services: []
`
	filename := filepath.Join(control, ".rungrid.yaml")
	if err := os.WriteFile(filename, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := manifest.Load(filename, "")
	if err != nil {
		t.Fatal(err)
	}
	plan := Build(loaded, "test")
	if plan.WorkspaceRoot != ".." || len(plan.Lifecycle.BeforeUp) != 1 || len(plan.Lifecycle.AfterDown) != 1 {
		t.Fatalf("unexpected lifecycle plan: %#v", plan.Lifecycle)
	}
	if plan.Lifecycle.BeforeUp[0].Argv[1] != "prepare" || plan.Lifecycle.AfterDown[0].Timeout != "6s" {
		t.Fatalf("lifecycle details changed: %#v", plan.Lifecycle)
	}
	if !contains(plan.Executables, "example-tool") {
		t.Fatalf("lifecycle executable missing from plan: %#v", plan.Executables)
	}
	content, err := plan.JSON()
	if err != nil {
		t.Fatal(err)
	}
	var human bytes.Buffer
	plan.WriteHuman(&human, present.New(false))
	combined := string(content) + human.String()
	if strings.Contains(combined, root) {
		t.Fatal("plan persisted an absolute developer path")
	}
	if !strings.Contains(human.String(), `argv=["example-tool" "prepare" "--token" "<redacted>"]`) {
		t.Fatalf("human plan omitted exact argv: %s", human.String())
	}
	if strings.Contains(combined, "private-value") {
		t.Fatal("plan exposed a secret-like lifecycle argument")
	}
}

func TestMultiWorkspacePlanGolden(t *testing.T) {
	t.Parallel()
	loaded, err := manifest.Load(filepath.Join("..", "..", "testdata", "multi-workspace", "control", ".rungrid.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	plan := Build(loaded, "dev")
	plan.Recovery = &RecoveryPlan{Action: "start", ManifestCompatible: true, LifecycleCompatible: true}
	var actual bytes.Buffer
	plan.WriteHuman(&actual, present.New(false))
	expected, err := os.ReadFile(filepath.Join("testdata", "multi-workspace-plan.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if actual.String() != string(expected) {
		t.Fatalf("plan golden differs\n--- actual ---\n%s\n--- expected ---\n%s", actual.String(), expected)
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
