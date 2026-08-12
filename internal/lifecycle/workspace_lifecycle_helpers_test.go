//go:build darwin || linux

package lifecycle

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
	"github.com/jamesonstone/rungrid/internal/supervisor"
	"github.com/jamesonstone/rungrid/internal/workspace"
	"gopkg.in/yaml.v3"
)

func lifecycleFixture(
	t *testing.T,
	root string,
	before []manifest.LifecycleCommand,
	after []manifest.LifecycleCommand,
) (state.Layout, workspace.Journal) {
	t.Helper()
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	configuration := manifest.Manifest{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Project:    manifest.Project{Name: "Example", Slug: "example", ID: "example-k7m4q2"},
		Workspace:  manifest.Workspace{Root: "."},
		Terminal:   manifest.Terminal{Mode: "headless"},
		Lifecycle:  manifest.Lifecycle{BeforeUp: before, AfterDown: after},
		Services:   []manifest.Service{},
	}
	configuration.ApplyDefaults()
	if err := manifest.Validate(&configuration, resolvedRoot); err != nil {
		t.Fatal(err)
	}
	content, err := yaml.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	layout, err := state.NewLayout(configuration.Project.ID, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	generation := "test-generation"
	manifestRelative := filepath.Join("generations", generation, "manifest.yaml")
	if err := state.WriteFileAtomic(layout.ProjectDir, manifestRelative, content, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteFileAtomic(layout.ProjectDir, filepath.Join("generations", generation, "process-compose.yaml"), []byte("process compose\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := state.WriteFileAtomic(layout.ProjectDir, "current", []byte(generation+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	journal := workspace.NewJournal(
		layout.ProjectID,
		generation,
		state.Hash(content),
		workspace.LifecycleDigest(configuration.Lifecycle),
		resolvedRoot,
		len(before) > 0 || len(after) > 0,
	)
	if err := workspace.WriteJournal(layout, journal); err != nil {
		t.Fatal(err)
	}
	return layout, journal
}

func completeRuntimeScope(t *testing.T, layout state.Layout, generationID string, runtimeState *supervisor.Runtime) {
	t.Helper()
	manifestContent, err := os.ReadFile(filepath.Join(layout.ProjectDir, "generations", generationID, "manifest.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	configuration := filepath.Join(layout.ProjectDir, "generations", generationID, "process-compose.yaml")
	configurationContent, err := os.ReadFile(configuration)
	if err != nil {
		t.Fatal(err)
	}
	runtimeState.EffectiveManifestSHA256 = state.Hash(manifestContent)
	runtimeState.Configuration = configuration
	runtimeState.ConfigurationHash = state.Hash(configurationContent)
	runtimeState.SocketOwnerUID = uint32(os.Getuid())
}

func lifecycleCommand(name string, argv ...string) manifest.LifecycleCommand {
	return manifest.LifecycleCommand{
		Name: name, WorkingDirectory: ".", Run: manifest.Command{Argv: argv},
	}
}

func writeExecutable(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

func assertLines(t *testing.T, filename string, expected []string) {
	t.Helper()
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	actual := strings.Fields(string(content))
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("lines = %#v, want %#v", actual, expected)
	}
}
