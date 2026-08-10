package manifest

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLoadExample(t *testing.T) {
	t.Parallel()
	loaded, err := Load(filepath.Join("..", "..", "testdata", "example", ".rungrid.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Manifest.Project.ID != "example-workspace-k7m4q2" {
		t.Fatalf("unexpected project id %q", loaded.Manifest.Project.ID)
	}
	if len(loaded.Manifest.Services) != 4 {
		t.Fatalf("got %d services", len(loaded.Manifest.Services))
	}
	api, _ := FindService(&loaded.Manifest, "api")
	if !reflect.DeepEqual(api.Terminal.TriggerArgv, []string{"make", "dev"}) {
		t.Fatalf("unexpected trigger %#v", api.Terminal.TriggerArgv)
	}
	web, _ := FindService(&loaded.Manifest, "web")
	if !reflect.DeepEqual(web.Terminal.TriggerArgv, web.Run.Argv) {
		t.Fatalf("default trigger %#v does not match run argv %#v", web.Terminal.TriggerArgv, web.Run.Argv)
	}
}

func TestLocalOverlayMergesServicesByName(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustMkdir(t, filepath.Join(root, "api"))
	base := `api_version: rungrid/v1
kind: Workspace
project: {name: Example, slug: example, id: example-k7m4q2}
terminal: {mode: headless}
services:
  - name: api
    source: native
    activation: tab
    working_directory: api
    run: {argv: [go, run, .]}
    environment:
      values: {A: one, B: base}
`
	overlay := `api_version: rungrid/v1
kind: Workspace
services:
  - name: api
    environment:
      values: {B: local, C: three}
`
	mustWrite(t, filepath.Join(root, ".rungrid.yaml"), base)
	mustWrite(t, filepath.Join(root, ".rungrid.local.yaml"), overlay)
	loaded, err := Load(filepath.Join(root, ".rungrid.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	values := loaded.Manifest.Services[0].Environment.Values
	if !reflect.DeepEqual(values, map[string]string{"A": "one", "B": "local", "C": "three"}) {
		t.Fatalf("unexpected merged values %#v", values)
	}
}

func TestLoadRejectsUnknownField(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, ".rungrid.yaml"), `api_version: rungrid/v1
kind: Workspace
project: {name: Example, slug: example, id: example-k7m4q2}
services: []
unknown: true
`)
	_, err := Load(filepath.Join(root, ".rungrid.yaml"), "")
	if err == nil || !strings.Contains(err.Error(), "field unknown not found") {
		t.Fatalf("expected strict unknown-field error, got %v", err)
	}
}

func TestLoadRejectsEscapingImport(t *testing.T) {
	t.Parallel()
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	mustMkdir(t, root)
	mustWrite(t, filepath.Join(parent, "outside.yaml"), "services: []\n")
	mustWrite(t, filepath.Join(root, ".rungrid.yaml"), `api_version: rungrid/v1
kind: Workspace
project: {name: Example, slug: example, id: example-k7m4q2}
imports: [../outside.yaml]
services: []
`)
	_, err := Load(filepath.Join(root, ".rungrid.yaml"), "")
	if err == nil || !strings.Contains(err.Error(), "escapes workspace") {
		t.Fatalf("expected import boundary error, got %v", err)
	}
}

func TestDependencyCycleRejected(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	m := Manifest{
		APIVersion: APIVersion,
		Kind:       Kind,
		Project:    Project{Name: "Example", Slug: "example", ID: "example-k7m4q2"},
		Services: []Service{
			{Name: "api", Source: "native", Activation: "workspace", WorkingDirectory: ".", Run: &Run{Argv: []string{"true"}}, DependsOn: map[string]string{"web": "running"}},
			{Name: "web", Source: "native", Activation: "workspace", WorkingDirectory: ".", Run: &Run{Argv: []string{"true"}}, DependsOn: map[string]string{"api": "running"}},
		},
	}
	m.ApplyDefaults()
	err := Validate(&m, root)
	if err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("expected dependency cycle, got %v", err)
	}
}

func TestLiteralSecretLikeEnvironmentKeyRejected(t *testing.T) {
	t.Parallel()
	m := Manifest{
		APIVersion: APIVersion,
		Kind:       Kind,
		Project:    Project{Name: "Example", Slug: "example", ID: "example-k7m4q2"},
		Services: []Service{{
			Name: "api", Source: "native", Activation: "workspace", WorkingDirectory: ".",
			Run: &Run{Argv: []string{"true"}}, Environment: Environment{Values: map[string]string{"API_TOKEN": "literal"}},
		}},
	}
	m.ApplyDefaults()
	err := Validate(&m, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "execution-time environment provider") {
		t.Fatalf("expected literal secret rejection, got %v", err)
	}
}

func TestProcessComposeLogLevelValidation(t *testing.T) {
	t.Parallel()
	for _, level := range []string{"trace", "debug", "info", "warn", "error", "fatal", "panic", "disabled"} {
		configuration := validEmptyManifest(level)
		if err := Validate(&configuration, t.TempDir()); err != nil {
			t.Errorf("accepted Process Compose log level %q: %v", level, err)
		}
	}
	configuration := validEmptyManifest("warning")
	if err := Validate(&configuration, t.TempDir()); err == nil || !strings.Contains(err.Error(), "runtime.process_compose.log_level") {
		t.Fatalf("expected invalid Process Compose log-level error, got %v", err)
	}
}

func TestProcessComposeLogRotationDefaultsAndValidation(t *testing.T) {
	t.Parallel()
	configuration := validEmptyManifest("info")
	if configuration.Runtime.ProcessCompose.LogRotation.MaxSizeMB != 10 || configuration.Runtime.ProcessCompose.LogRotation.MaxBackups != 1 {
		t.Fatalf("unexpected log rotation defaults: %#v", configuration.Runtime.ProcessCompose.LogRotation)
	}
	configuration.Runtime.ProcessCompose.LogRotation.MaxSizeMB = -1
	configuration.Runtime.ProcessCompose.LogRotation.MaxBackups = -1
	err := Validate(&configuration, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "log_rotation.max_size_mb") || !strings.Contains(err.Error(), "log_rotation.max_backups") {
		t.Fatalf("expected invalid log rotation error, got %v", err)
	}
}

func validEmptyManifest(logLevel string) Manifest {
	configuration := Manifest{
		APIVersion: APIVersion,
		Kind:       Kind,
		Project:    Project{Name: "Example", Slug: "example", ID: "example-k7m4q2"},
	}
	configuration.ApplyDefaults()
	configuration.Runtime.ProcessCompose.LogLevel = logLevel
	return configuration
}

func FuzzSlug(f *testing.F) {
	for _, seed := range []string{"Example Workspace", "API_and Web", "a/b", "", "éxample"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		result := Slug(input)
		if strings.ContainsAny(result, "/\\ \t\n") {
			t.Fatalf("unsafe slug %q", result)
		}
		if result != strings.Trim(result, "-") {
			t.Fatalf("slug has boundary hyphen %q", result)
		}
	})
}

func mustWrite(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.WriteFile(filename, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, directory string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
}
