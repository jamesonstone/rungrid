package processcompose

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/subprocess"

	"github.com/jamesonstone/rungrid/internal/manifest"
)

func TestCompileMatchesGoldenAndProcessComposeSchema(t *testing.T) {
	t.Parallel()
	loaded, err := manifest.Load(filepath.Join("..", "..", "testdata", "example", ".rungrid.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := Compile(&loaded.Manifest, "0123456789abcdef")
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join("testdata", "example-process-compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(compiled.Configuration) != string(expected) {
		t.Fatalf("Process Compose output differs from golden\n--- got ---\n%s\n--- want ---\n%s", compiled.Configuration, expected)
	}
	if strings.Contains(string(compiled.Configuration), loaded.WorkspaceRoot) {
		t.Fatal("generated configuration persisted an absolute workspace path")
	}
	if !strings.Contains(string(compiled.Wrappers["api"]), "internal exec-service --project-id example-workspace-k7m4q2") {
		t.Fatalf("unexpected api wrapper:\n%s", compiled.Wrappers["api"])
	}
	if wrapper := string(compiled.Wrappers["rungrid-maintenance-sync"]); !strings.Contains(wrapper, "internal maintenance-worker") || !strings.Contains(wrapper, "--operation sync") {
		t.Fatalf("unexpected maintenance wrapper:\n%s", wrapper)
	}
	processCompose, err := exec.LookPath("process-compose")
	if err != nil {
		t.Skip("Process Compose is not installed")
	}
	directory := t.TempDir()
	filename := filepath.Join(directory, "process-compose.yaml")
	if err := os.WriteFile(filename, compiled.Configuration, 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(processCompose, "-f", filename, "--dry-run")
	command.Dir = directory
	if output, err := subprocess.Combined(command); err != nil {
		t.Fatalf("minimum-version schema rejected generated config: %v\n%s", err, output)
	}
}

func TestClientUsesExactUnixSocketArguments(t *testing.T) {
	directory := t.TempDir()
	logPath := filepath.Join(directory, "arguments.log")
	executable := filepath.Join(directory, "process-compose")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$FAKE_PROCESS_COMPOSE_LOG"
printf '{"level":"debug","message":"diagnostic"}\n' >&2
printf '[{"name":"api","status":"Running","pid":42}]\n'
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_PROCESS_COMPOSE_LOG", logPath)
	client := Client{Executable: executable, Socket: "runtime.sock", LogFile: filepath.Join(directory, "client.log"), WorkDir: directory}
	states, raw, err := client.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(states, []ProcessState{{Name: "api", Status: "Running", PID: 42}}) {
		t.Fatalf("unexpected states %#v", states)
	}
	if string(raw) != `[{"name":"api","status":"Running","pid":42}]` {
		t.Fatalf("stderr contaminated raw JSON: %q", raw)
	}
	arguments, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := "-U\n-u\nruntime.sock\n-L\n" + filepath.Join(directory, "client.log") + "\nlist\n--output\njson\n"
	if string(arguments) != want {
		t.Fatalf("unexpected arguments\n got: %q\nwant: %q", arguments, want)
	}
}

func TestSupportedVersionRange(t *testing.T) {
	t.Parallel()
	for version, expected := range map[string]bool{"v1.119.9": false, "v1.120.0": true, "v1.999.0": true, "v2.0.0": false, "invalid": false} {
		if actual := SupportedVersion(version); actual != expected {
			t.Errorf("SupportedVersion(%q)=%v, want %v", version, actual, expected)
		}
	}
}

func TestInternalLogIsDiscarded(t *testing.T) {
	t.Parallel()
	if InternalLog() != os.DevNull {
		t.Fatalf("internal Process Compose logs must be discarded, got %q", InternalLog())
	}
}
