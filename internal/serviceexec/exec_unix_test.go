//go:build darwin || linux

package serviceexec

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

func TestExecUsesConfiguredWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	workingDirectory := filepath.Join(root, "services", "api")
	if err := os.MkdirAll(workingDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "directory.log")
	executable := filepath.Join(root, "record-directory")
	script := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$PWD\" > \"$RUNGRID_DIR_LOG\"\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	command := exec.Command(os.Args[0], "-test.run=TestExecHelperProcess")
	command.Env = append(os.Environ(),
		"RUNGRID_EXEC_HELPER=1",
		"RUNGRID_EXEC_ROOT="+root,
		"RUNGRID_EXECUTABLE="+executable,
		"RUNGRID_DIR_LOG="+logPath,
	)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("service execution helper failed: %v\n%s", err, output)
	}

	directory, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	resolvedDirectory, err := filepath.EvalSymlinks(workingDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(directory)) != resolvedDirectory {
		t.Fatalf("service ran in %q, want %q", strings.TrimSpace(string(directory)), resolvedDirectory)
	}
}

func TestExecHelperProcess(t *testing.T) {
	if os.Getenv("RUNGRID_EXEC_HELPER") != "1" {
		return
	}
	root := os.Getenv("RUNGRID_EXEC_ROOT")
	service := manifest.Service{
		Name:             "api",
		Source:           "native",
		WorkingDirectory: filepath.Join("services", "api"),
		Run:              &manifest.Run{Argv: []string{os.Getenv("RUNGRID_EXECUTABLE")}},
	}
	runtimeContext := Context{WorkspaceRoot: root, Manifest: &manifest.Manifest{Services: []manifest.Service{service}}}
	if err := Exec(context.Background(), runtimeContext, service.Name); err != nil {
		t.Fatal(err)
	}
	t.Fatal("service execution returned without replacing the helper process")
}

func TestComposeShutdownUsesExactConfiguredArguments(t *testing.T) {
	root := t.TempDir()
	backend := filepath.Join(root, "backend")
	if err := os.MkdirAll(backend, 0o700); err != nil {
		t.Fatal(err)
	}
	resolvedBackend, err := filepath.EvalSymlinks(backend)
	if err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(root, "arguments.log")
	directoryLog := filepath.Join(root, "directory.log")
	executable := filepath.Join(root, "fake-compose")
	script := "#!/bin/sh\nset -eu\nprintf '%s\\n' \"$@\" > \"$RUNGRID_ARG_LOG\"\nprintf '%s\\n' \"$PWD\" > \"$RUNGRID_DIR_LOG\"\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	service := &manifest.Service{
		Name:             "database",
		Repository:       "backend",
		Source:           "compose",
		WorkingDirectory: ".",
		Environment: manifest.Environment{Values: map[string]string{
			"RUNGRID_ARG_LOG": logPath,
			"RUNGRID_DIR_LOG": directoryLog,
		}},
		Compose: &manifest.Compose{
			File:        "compose.yaml",
			ProjectName: "example-workspace",
			Service:     "database",
			Profiles:    []string{"development", "metrics"},
			DownArgv:    []string{executable, "compose"},
		},
	}

	m := &manifest.Manifest{Repositories: map[string]manifest.Repository{"backend": {Path: "backend"}}, Services: []manifest.Service{*service}}
	if err := ComposeShutdown(context.Background(), state.Layout{}, m, service, root); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"compose",
		"--file", "compose.yaml",
		"--project-name", "example-workspace",
		"--profile", "development",
		"--profile", "metrics",
		"stop", "database",
		"",
	}, "\n")
	if string(content) != want {
		t.Fatalf("unexpected arguments:\n%s\nwant:\n%s", content, want)
	}
	directory, err := os.ReadFile(directoryLog)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(directory)) != resolvedBackend {
		t.Fatalf("Compose ran in %q, want %q", strings.TrimSpace(string(directory)), resolvedBackend)
	}
}
