package processcompose

import (
	"context"
	"net/http"
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

func TestFiniteClientDoesNotExecuteProcessComposeBinary(t *testing.T) {
	directory, err := os.MkdirTemp("/tmp", "rg-pc-binary-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	logPath := filepath.Join(directory, "arguments.log")
	executable := filepath.Join(directory, "process-compose")
	script := `#!/bin/sh
printf '%s\n' "$@" > "$FAKE_PROCESS_COMPOSE_LOG"
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FAKE_PROCESS_COMPOSE_LOG", logPath)
	client := serveUnixClient(t, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/processes" {
			t.Fatalf("unexpected request path %q", request.URL.Path)
		}
		_, _ = w.Write([]byte(`[{"name":"api","status":"Running","pid":42}]`))
	}))
	client.Executable = executable
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
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("finite control call executed Process Compose binary: %v", err)
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
