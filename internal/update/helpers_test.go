package update

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/maintenance"
	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

type fixture struct {
	t       *testing.T
	primary string
	writer  string
	layout  state.Layout
	coord   *fakeCoordinator
	exec    *fakeExecutor
}

var fixtureFiles = map[string]string{
	".gitignore":                 "node_modules/\n",
	"backend/go.mod":             "module example.com/backend\n",
	"backend/go.sum":             "",
	"backend/main.go":            "package main\n",
	"backend/migrations/001.sql": "create table one();\n",
	"frontend/package.json":      `{"scripts":{"start":"expo start"}}` + "\n",
	"frontend/package-lock.json": "{}\n",
	"frontend/app.tsx":           "export {}\n",
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	primary := filepath.Join(root, "primary")
	writer := filepath.Join(root, "writer")
	gitRun(t, root, "init", "--bare", "--initial-branch=main", remote)
	gitRun(t, root, "init", "--initial-branch=main", primary)
	configure(t, primary)
	for name, content := range fixtureFiles {
		writeFile(t, filepath.Join(primary, name), content)
	}
	gitRun(t, primary, "add", ".")
	gitRun(t, primary, "commit", "-m", "initial")
	gitRun(t, primary, "remote", "add", "origin", remote)
	gitRun(t, primary, "push", "-u", "origin", "main")
	gitRun(t, root, "clone", remote, writer)
	configure(t, writer)
	if err := os.MkdirAll(filepath.Join(primary, "frontend", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	primary, _ = filepath.EvalSymlinks(primary)
	layout, err := state.NewLayout("example-k7m4q2", filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	backend := &manifest.Service{Name: "backend", Source: "native", Run: &manifest.Run{Argv: []string{"go", "run", "."}},
		Refresh: &manifest.Refresh{Steps: []manifest.RefreshStep{{Name: "migrate", Run: manifest.Command{Argv: []string{"make", "migrate"}}, Watch: []string{"migrations"}}}}}
	frontend := &manifest.Service{Name: "frontend", Source: "native", Activation: "tab", Run: &manifest.Run{Argv: []string{"npm", "run", "start"}}}
	coordinator := &fakeCoordinator{services: []Service{
		{Config: backend, WorkingDirectory: filepath.Join(primary, "backend"), Running: true, RestartPath: RestartProcess},
		{Config: frontend, WorkingDirectory: filepath.Join(primary, "frontend"), Running: true, RestartPath: RestartSession},
	}}
	return &fixture{t: t, primary: primary, writer: writer, layout: layout, coord: coordinator, exec: &fakeExecutor{}}
}

func (f *fixture) push(name, content string) {
	f.t.Helper()
	writeFile(f.t, filepath.Join(f.writer, name), content)
	gitRun(f.t, f.writer, "add", name)
	gitRun(f.t, f.writer, "commit", "-m", "change "+name)
	gitRun(f.t, f.writer, "push", "origin", "main")
}

func (f *fixture) repository() maintenance.Repository {
	return maintenance.Repository{Name: "app", Path: f.primary, TopLevel: f.primary, Remote: "origin"}
}

func (f *fixture) run(options Options) Report {
	f.t.Helper()
	f.coord.paused, f.coord.resumed, f.coord.restarted, f.exec.commands = nil, false, nil, nil
	report, err := runRepositories(context.Background(), []maintenance.Repository{f.repository()}, nil, 0, options,
		Dependencies{Layout: f.layout, Coordinator: f.coord, Executor: f.exec})
	if err != nil && f.exec.fail == "" && f.coord.failPause == "" {
		f.t.Fatalf("update failed: %v\n%+v", err, report)
	}
	return report
}

func (f *fixture) head() string { return gitRun(f.t, f.primary, "rev-parse", "HEAD") }

// fakeCoordinator models Process Compose state: running services, pauses
// that can fail per service, and the resume order.
type fakeCoordinator struct {
	services  []Service
	paused    []string
	resumed   bool
	restarted []string
	failPause string
	// recover makes this service's resume succeed only through a fallback.
	recover string
}

func (c *fakeCoordinator) Services(context.Context, string) ([]Service, error) {
	return c.services, nil
}

func (c *fakeCoordinator) Pause(_ context.Context, name string) (maintenance.ResumeFunc, error) {
	if name == c.failPause {
		return nil, errors.New("tab-owned service has no live owning session")
	}
	c.setRunning(name, false)
	c.paused = append(c.paused, name)
	return func(context.Context) error {
		c.setRunning(name, true)
		c.resumed = true
		c.restarted = append(c.restarted, name)
		if name == c.recover {
			return Recovered{Detail: "session resume failed (broken pipe); restarted through Process Compose"}
		}
		return nil
	}, nil
}

func (c *fakeCoordinator) setRunning(name string, running bool) {
	for index := range c.services {
		if c.services[index].Config.Name == name {
			c.services[index].Running = running
		}
	}
}

func (c *fakeCoordinator) allRunning() bool {
	for _, service := range c.services {
		if !service.Running {
			return false
		}
	}
	return true
}

type fakeExecutor struct {
	commands []string
	fail     string
}

func (e *fakeExecutor) Execute(_ context.Context, _ int, command Command) (string, error) {
	e.commands = append(e.commands, command.Name)
	if command.Name == e.fail {
		return "", errors.New(command.Name + " exited 1")
	}
	return "lifecycle-logs/update/" + command.Name + ".log", nil
}

func serviceStatus(t *testing.T, result RepositoryResult, name string) ServiceResult {
	t.Helper()
	for _, service := range result.Services {
		if service.Name == name {
			return service
		}
	}
	t.Fatalf("service %s missing from %+v", name, result.Services)
	return ServiceResult{}
}

func gitRun(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func configure(t *testing.T, directory string) {
	t.Helper()
	gitRun(t, directory, "config", "user.name", "Example User")
	gitRun(t, directory, "config", "user.email", "example@example.com")
}

func writeFile(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
