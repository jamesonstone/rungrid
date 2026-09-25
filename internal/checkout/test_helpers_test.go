package checkout

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jamesonstone/rungrid/internal/manifest"
	"github.com/jamesonstone/rungrid/internal/state"
)

type repositoryFixture struct {
	root    string
	remote  string
	primary string
	lane    string
	writer  string
}

func newRepositoryFixture(t *testing.T) repositoryFixture {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	primary := filepath.Join(root, "primary")
	writer := filepath.Join(root, "writer")
	lane := filepath.Join(root, "GH-12")
	gitTest(t, root, "init", "--bare", "--initial-branch=main", remote)
	gitTest(t, root, "init", "--initial-branch=main", primary)
	configureGitUser(t, primary)
	writeTestFile(t, filepath.Join(primary, "README.md"), "initial\n")
	gitTest(t, primary, "add", "README.md")
	gitTest(t, primary, "commit", "-m", "initial")
	gitTest(t, primary, "remote", "add", "origin", remote)
	gitTest(t, primary, "push", "-u", "origin", "main")
	gitTest(t, root, "clone", remote, writer)
	configureGitUser(t, writer)
	gitTest(t, primary, "worktree", "add", "-b", "GH-12", lane, "origin/main")
	gitTest(t, lane, "push", "-u", "origin", "GH-12")
	return repositoryFixture{root: root, remote: remote, primary: primary, lane: lane, writer: writer}
}

func loadedServices(t *testing.T, fixture repositoryFixture) (*manifest.Loaded, state.Layout) {
	t.Helper()
	relative, err := filepath.Rel(fixture.root, fixture.primary)
	if err != nil {
		t.Fatal(err)
	}
	value := manifest.Manifest{
		APIVersion: manifest.APIVersion,
		Kind:       manifest.Kind,
		Project:    manifest.Project{Name: "Example", Slug: "example", ID: "example-k7m4q2"},
		Repositories: map[string]manifest.Repository{
			"api": {Path: relative, Remote: "origin"},
		},
		Terminal: manifest.Terminal{Mode: "headless"},
		Services: []manifest.Service{
			{Name: "api", Repository: "api", Source: "native", WorkingDirectory: "."},
			{Name: "worker", Repository: "api", Source: "native", WorkingDirectory: "."},
		},
	}
	value.ApplyDefaults()
	layout, err := state.NewLayout(value.Project.ID, filepath.Join(fixture.root, "state"))
	if err != nil {
		t.Fatal(err)
	}
	if err := layout.Ensure(); err != nil {
		t.Fatal(err)
	}
	return &manifest.Loaded{Manifest: value, WorkspaceRoot: fixture.root, ManifestDir: fixture.root}, layout
}

func (fixture repositoryFixture) advanceLane(t *testing.T, value string) string {
	t.Helper()
	gitTest(t, fixture.writer, "fetch", "origin")
	gitTest(t, fixture.writer, "checkout", "-B", "GH-12", "origin/GH-12")
	writeTestFile(t, filepath.Join(fixture.writer, "README.md"), value+"\n")
	gitTest(t, fixture.writer, "add", "README.md")
	gitTest(t, fixture.writer, "commit", "-m", value)
	gitTest(t, fixture.writer, "push", "origin", "GH-12")
	return gitTest(t, fixture.writer, "rev-parse", "HEAD")
}

func gitTest(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return strings.TrimSpace(string(output))
}

func configureGitUser(t *testing.T, directory string) {
	t.Helper()
	gitTest(t, directory, "config", "user.name", "Example User")
	gitTest(t, directory, "config", "user.email", "example@example.com")
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustPhysical(t *testing.T, path string) string {
	t.Helper()
	resolved, err := physicalPath(path)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}
