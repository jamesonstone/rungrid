// Package overridetest builds real Git workspaces for repository-override
// tests: an implicit-workspace manifest whose services share checkouts, a
// registered worktree, a same-origin clone, and foreign or plain directories.
package overridetest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

// Fixture paths are physical so they compare equal to resolved results.
type Fixture struct {
	Root      string
	Workspace string
	Service   string
	Other     string
	Lane      string
	Clone     string
	Foreign   string
	Plain     string
	Loaded    *manifest.Loaded
	Layout    state.Layout
}

// Manifest is the default fixture manifest. alpha and beta share the svc
// checkout, gamma runs from other, and db is an external service in svc.
const Manifest = `api_version: rungrid/v1
kind: Workspace
project:
  name: Override Fixture
  slug: override-fixture
  id: override-fixture-k7m4q2
terminal:
  mode: headless
  open: false
services:
  - name: alpha
    source: native
    activation: workspace
    working_directory: svc
    run:
      argv: [sh, ../tools/run.sh, alpha, --config=../svc/config.yaml, ./local.sh]
  - name: beta
    source: native
    activation: workspace
    working_directory: svc
    run:
      argv: [sh, ../tools/run.sh, beta]
  - name: gamma
    source: native
    activation: workspace
    working_directory: other
    run:
      argv: [sh, ../tools/run.sh, gamma]
  - name: db
    source: external
    working_directory: svc
    external:
      url: http://127.0.0.1:9/health
`

// New builds the fixture with the default manifest.
func New(t *testing.T) Fixture {
	t.Helper()
	return NewWithManifest(t, Manifest)
}

// NewWithManifest builds the fixture repositories and loads content as the
// workspace manifest.
func NewWithManifest(t *testing.T, content string) Fixture {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixture := Fixture{
		Root: root, Workspace: filepath.Join(root, "ws"),
		Lane: filepath.Join(root, "lanes", "GH-1"), Clone: filepath.Join(root, "clone"),
		Foreign: filepath.Join(root, "foreign"), Plain: filepath.Join(root, "plain"),
	}
	fixture.Service = filepath.Join(fixture.Workspace, "svc")
	fixture.Other = filepath.Join(fixture.Workspace, "other")
	WriteFile(t, filepath.Join(fixture.Workspace, "tools", "run.sh"), "#!/bin/sh\nexec sleep 1000\n")
	initRepository(t, fixture.Service, "https://github.com/acme/svc.git", "primary")
	initRepository(t, fixture.Other, "git@github.com:acme/other.git", "primary")
	Git(t, fixture.Service, "worktree", "add", "-q", "-b", "GH-1", fixture.Lane)
	initRepository(t, fixture.Clone, "git@github.com:Acme/svc", "clone")
	initRepository(t, fixture.Foreign, "https://github.com/acme/foreign.git", "foreign")
	if err := os.MkdirAll(fixture.Plain, 0o700); err != nil {
		t.Fatal(err)
	}
	WriteFile(t, filepath.Join(fixture.Workspace, ".rungrid.yaml"), content)
	loaded, err := manifest.Load(filepath.Join(fixture.Workspace, ".rungrid.yaml"), "")
	if err != nil {
		t.Fatal(err)
	}
	fixture.Loaded = loaded
	fixture.Layout, err = state.NewLayout(loaded.Manifest.Project.ID, filepath.Join(root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.Layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func initRepository(t *testing.T, directory, origin, version string) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	Git(t, directory, "init", "-q", "-b", "main")
	Git(t, directory, "config", "user.name", "Example User")
	Git(t, directory, "config", "user.email", "example@example.com")
	WriteFile(t, filepath.Join(directory, "VERSION"), version+"\n")
	Git(t, directory, "add", "VERSION")
	Git(t, directory, "commit", "-q", "-m", "initial "+version)
	Git(t, directory, "remote", "add", "origin", origin)
}

// Git runs git in directory and returns trimmed output.
func Git(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

// WriteFile writes content, creating parent directories.
func WriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}
