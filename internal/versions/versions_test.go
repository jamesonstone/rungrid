package versions

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/processcompose"
	"github.com/jamesonstone/rungrid/internal/subprocess"
	"github.com/jamesonstone/rungrid/internal/supervisor"
)

func TestListeningPortsParsesAndSortsBatchedLsofOutput(t *testing.T) {
	directory := t.TempDir()
	executable := filepath.Join(directory, "lsof")
	script := "#!/bin/sh\nprintf 'p42\\nn127.0.0.1:9000\\nn*:8080\\nn[::1]:9000\\np77\\nn*:7000\\n'\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	ports := listeningPortsByPID(context.Background(), []int{42, 77})
	if !reflect.DeepEqual(ports[42], []int{8080, 9000}) || !reflect.DeepEqual(ports[77], []int{7000}) {
		t.Fatalf("unexpected ports %#v", ports)
	}
}

func TestCollectorCachesExpensiveDiscoveryIndependently(t *testing.T) {
	workspace := t.TempDir()
	clientExecutable := filepath.Join(workspace, "process-compose")
	script := "#!/bin/sh\nprintf '[{\"name\":\"api\",\"status\":\"Running\",\"pid\":42}]\\n'\n"
	if err := os.WriteFile(clientExecutable, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Services: []manifest.Service{
			{Name: "api", Source: "native", WorkingDirectory: "."},
			{Name: "database", Source: "external", WorkingDirectory: ".", External: &manifest.External{}},
		},
	}
	now := time.Unix(100, 0)
	sourceCalls, listenerCalls, externalCalls := 0, 0, 0
	collector := NewCollector()
	collector.now = func() time.Time { return now }
	collector.captureSource = func(context.Context, string, string) sourceVersion {
		sourceCalls++
		return sourceVersion{branch: "main", commit: "1234567", gitState: "clean", worktree: "workspace", root: workspace}
	}
	collector.captureListeners = func(context.Context, []int) map[int][]int {
		listenerCalls++
		return map[int][]int{42: {8000}}
	}
	collector.checkExternal = func(context.Context, *manifest.Manifest, string, *manifest.Service) error {
		externalCalls++
		return errors.New("not ready")
	}
	client := processcompose.Client{Executable: clientExecutable, Socket: "socket", LogFile: os.DevNull, WorkDir: workspace}
	runtimeState := supervisor.Runtime{WorkspaceRoot: workspace, GenerationID: "generation"}

	first := collector.Capture(context.Background(), m, runtimeState, client)
	now = now.Add(time.Second)
	second := collector.Capture(context.Background(), m, runtimeState, client)
	if sourceCalls != 1 || listenerCalls != 1 || externalCalls != 1 {
		t.Fatalf("unexpected calls after cached capture: source=%d listener=%d external=%d", sourceCalls, listenerCalls, externalCalls)
	}
	if !MateriallyEqual(first, second) {
		t.Fatalf("timestamps should not make snapshots materially different: %#v %#v", first, second)
	}
	now = now.Add(5 * time.Second)
	collector.Capture(context.Background(), m, runtimeState, client)
	if sourceCalls != 1 || listenerCalls != 2 || externalCalls != 2 {
		t.Fatalf("unexpected calls after five seconds: source=%d listener=%d external=%d", sourceCalls, listenerCalls, externalCalls)
	}
	now = now.Add(5 * time.Second)
	collector.Capture(context.Background(), m, runtimeState, client)
	if sourceCalls != 2 || listenerCalls != 3 || externalCalls != 3 {
		t.Fatalf("unexpected calls after ten seconds: source=%d listener=%d external=%d", sourceCalls, listenerCalls, externalCalls)
	}
}

func TestGitVersionReportsBranchCommitCleanDirtyAndWorktree(t *testing.T) {
	t.Parallel()
	directory := t.TempDir()
	runGitTest(t, directory, "init", "-b", "main")
	runGitTest(t, directory, "config", "user.name", "Example User")
	runGitTest(t, directory, "config", "user.email", "example@example.invalid")
	filename := filepath.Join(directory, "README.md")
	if err := os.WriteFile(filename, []byte("example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitTest(t, directory, "add", "README.md")
	runGitTest(t, directory, "commit", "-m", "initial")
	branch, commit, stateValue, worktree := gitVersion(context.Background(), directory)
	if branch != "main" || commit == "" || stateValue != "clean" || worktree != filepath.Base(directory) {
		t.Fatalf("unexpected clean Git version branch=%q commit=%q state=%q worktree=%q", branch, commit, stateValue, worktree)
	}
	if err := os.WriteFile(filename, []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, stateValue, _ = gitVersion(context.Background(), directory)
	if stateValue != "dirty" {
		t.Fatalf("expected dirty state, got %q", stateValue)
	}
}

func TestCaptureUsesEachServiceRepository(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	for _, name := range []string{"backend", "frontend"} {
		directory := filepath.Join(workspace, name)
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
		runGitTest(t, directory, "init", "-b", "main")
		runGitTest(t, directory, "config", "user.name", "Example User")
		runGitTest(t, directory, "config", "user.email", "example@example.invalid")
		if err := os.WriteFile(filepath.Join(directory, "README.md"), []byte(name+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		runGitTest(t, directory, "add", "README.md")
		runGitTest(t, directory, "commit", "-m", "initial")
	}
	clientExecutable := filepath.Join(workspace, "process-compose")
	if err := os.WriteFile(clientExecutable, []byte("#!/bin/sh\nprintf '[{\"name\":\"backend\",\"status\":\"Running\"},{\"name\":\"frontend\",\"status\":\"Running\"}]\\n'\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	m := &manifest.Manifest{
		Repositories: map[string]manifest.Repository{
			"backend":  {Path: "backend"},
			"frontend": {Path: "frontend"},
		},
		Services: []manifest.Service{
			{Name: "backend", Repository: "backend", Source: "native", WorkingDirectory: "."},
			{Name: "frontend", Repository: "frontend", Source: "native", WorkingDirectory: "."},
		},
	}
	snapshot := Capture(context.Background(), m, supervisor.Runtime{WorkspaceRoot: workspace, GenerationID: "generation"}, processcompose.Client{
		Executable: clientExecutable, Socket: "socket", LogFile: "log", WorkDir: workspace,
	})
	if len(snapshot.Services) != 2 {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
	for _, service := range snapshot.Services {
		if service.Repository != service.Name || service.Worktree != service.Name || service.GitState != "clean" {
			t.Fatalf("service repository state changed: %#v", service)
		}
	}
}

func runGitTest(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	if output, err := subprocess.Combined(command); err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
}
